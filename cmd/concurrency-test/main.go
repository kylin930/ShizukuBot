// Command concurrency-test verifies the code-plugin HTTP path with capture-only replies.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	url := flag.String("url", "http://127.0.0.1:4545", "local test server")
	user := flag.String("u", "admin", "login name")
	password := flag.String("p", "", "login password")
	bot := flag.String("b", "sandbox", "test bot scope")
	concurrency := flag.Int("c", 10, "concurrency")
	total := flag.Int("n", 100, "request count")
	flag.Parse()
	if *password == "" || *concurrency < 1 || *concurrency > 32 || *total < 1 {
		fmt.Fprintln(os.Stderr, "password, positive count and concurrency <=32 required")
		os.Exit(1)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	cookie := ""
	request := func(method, path string, body any) (json.RawMessage, error) {
		var data []byte
		var err error
		if body != nil {
			data, err = json.Marshal(body)
			if err != nil {
				return nil, err
			}
		}
		req, err := http.NewRequest(method, strings.TrimRight(*url, "/")+path, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if err != nil {
			return nil, err
		}
		if response.StatusCode != 200 {
			return nil, fmt.Errorf("HTTP %d", response.StatusCode)
		}
		if path == "/login" {
			if len(response.Cookies()) == 0 {
				return nil, fmt.Errorf("login had no session")
			}
			cookie = response.Cookies()[0].String()
		}
		var envelope struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &envelope) != nil {
			return nil, fmt.Errorf("invalid JSON response")
		}
		if envelope.Code != 0 {
			return nil, fmt.Errorf("API: %s", envelope.Message)
		}
		return raw, nil
	}
	fatal := func(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	if _, err := request("POST", "/login", map[string]string{"username": *user, "password": *password}); err != nil {
		fatal(err)
	}
	id := "hello" // compiled catalog entry; this helper never installs or changes plugin code.
	var success, failed atomic.Int32
	var wg sync.WaitGroup
	jobs := make(chan int)
	messages := []string{"/ping", "/ping hello", "/unknown"}
	for range *concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				message := messages[i%len(messages)]
				raw, err := request("POST", "/api/bots/"+*bot+"/sandbox/send", map[string]string{"type": "c2c", "content": message, "plugin_id": id})
				var result struct {
					Replies []string `json:"replies"`
					Errors  []string `json:"errors"`
				}
				if err == nil {
					err = json.Unmarshal(raw, &result)
				}
				want := 1
				if message == "/unknown" {
					want = 0
				}
				if err != nil || len(result.Errors) > 0 || len(result.Replies) != want || (want == 1 && result.Replies[0] != "pong") {
					failed.Add(1)
				} else {
					success.Add(1)
				}
			}
		}()
	}
	for i := 0; i < *total; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	fmt.Printf("success=%d failed=%d total=%d\n", success.Load(), failed.Load(), *total)
	if failed.Load() > 0 {
		os.Exit(1)
	}
}
