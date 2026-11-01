// Command concurrency-test verifies the code-plugin HTTP path with capture-only replies.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
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
	cleanup := flag.Bool("cleanup", true, "remove test package")
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
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		fatal(err)
	}
	id := "http-" + hex.EncodeToString(random[:])
	manifest := map[string]any{"api_version": 1, "id": id, "name": "HTTP regression", "version": "1.0.0", "entry": "dist/index.js", "runtime": "javascript", "permissions": []string{"commands.register", "message.send"}, "config_schema": map[string]any{"type": "object"}}
	meta, _ := json.Marshal(manifest)
	code := fmt.Sprintf(`export default {id:%q,version:'1.0.0',commands:{ping:async c=>c.reply('pong'),echo:async c=>c.reply(c.args.join(' ')),time:async c=>c.reply('time'),add:async c=>c.reply('sum')}};`, id)
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	for name, b := range map[string][]byte{"manifest.json": meta, "dist/index.js": []byte(code)} {
		w, err := z.Create(name)
		if err != nil {
			fatal(err)
		}
		if _, err = w.Write(b); err != nil {
			fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		fatal(err)
	}
	if _, err := request("POST", "/api/plugins", map[string]string{"package_base64": base64.StdEncoding.EncodeToString(archive.Bytes())}); err != nil {
		fatal(err)
	}
	if _, err := request("PUT", "/api/plugins/"+id, map[string]any{"bot_id": *bot, "trusted": true}); err != nil {
		fatal(err)
	}
	var success, failed atomic.Int32
	var wg sync.WaitGroup
	jobs := make(chan int)
	messages := []string{"/ping", "/echo hello world", "/time", "/add 1 2", "/unknown"}
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
				if err != nil || len(result.Errors) > 0 || len(result.Replies) != want {
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
	if *cleanup {
		if _, err := request("DELETE", "/api/plugins/"+id, nil); err != nil {
			fatal(err)
		}
		fmt.Println("test package removed")
	}
	if failed.Load() > 0 {
		os.Exit(1)
	}
}
