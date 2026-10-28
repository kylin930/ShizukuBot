package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// 测试插件 JSON：包含 /ping /echo /time /add 四个指令
const testPluginJSON = `{
  "id": "plugin_concurrency_test",
  "name": "并发测试插件",
  "description": "用于并发指令解析测试",
  "enabled": true,
  "bot_id": "*",
  "graph": {
    "last_node_id": 8,
    "last_link_id": 4,
    "nodes": [
      {
        "id": 1, "type": "event/on_command",
        "pos": [100, 100], "size": {"0": 210, "1": 182},
        "flags": {}, "order": 0, "mode": 0,
        "outputs": [
          {"name": "触发", "type": -1, "links": [1], "slot_index": 0},
          {"name": "指令名", "type": "string", "links": null},
          {"name": "参数列表", "type": "array", "links": null},
          {"name": "原始消息", "type": "string", "links": null},
          {"name": "发送者ID", "type": "string", "links": null},
          {"name": "来源ID", "type": "string", "links": null}
        ],
        "properties": {"command": "ping", "prefix": "/"},
        "boxcolor": "#3a3"
      },
      {
        "id": 2, "type": "action/send_message",
        "pos": [500, 100], "size": {"0": 210, "1": 78},
        "flags": {}, "order": 1, "mode": 0,
        "inputs": [
          {"name": "触发", "type": -1, "link": 1},
          {"name": "回复内容", "type": "string", "link": null}
        ],
        "outputs": [{"name": "完成", "type": -1, "links": null}],
        "properties": {"template": "pong! 🏓"},
        "boxcolor": "#33a"
      },
      {
        "id": 3, "type": "event/on_command",
        "pos": [100, 350], "size": {"0": 210, "1": 182},
        "flags": {}, "order": 2, "mode": 0,
        "outputs": [
          {"name": "触发", "type": -1, "links": [2], "slot_index": 0},
          {"name": "指令名", "type": "string", "links": null},
          {"name": "参数列表", "type": "array", "links": null},
          {"name": "原始消息", "type": "string", "links": null},
          {"name": "发送者ID", "type": "string", "links": null},
          {"name": "来源ID", "type": "string", "links": null}
        ],
        "properties": {"command": "echo", "prefix": "/"},
        "boxcolor": "#3a3"
      },
      {
        "id": 4, "type": "action/send_message",
        "pos": [500, 350], "size": {"0": 210, "1": 78},
        "flags": {}, "order": 3, "mode": 0,
        "inputs": [
          {"name": "触发", "type": -1, "link": 2},
          {"name": "回复内容", "type": "string", "link": null}
        ],
        "outputs": [{"name": "完成", "type": -1, "links": null}],
        "properties": {"template": "echo 回复 ✅"},
        "boxcolor": "#33a"
      },
      {
        "id": 5, "type": "event/on_command",
        "pos": [100, 600], "size": {"0": 210, "1": 182},
        "flags": {}, "order": 4, "mode": 0,
        "outputs": [
          {"name": "触发", "type": -1, "links": [3], "slot_index": 0},
          {"name": "指令名", "type": "string", "links": null},
          {"name": "参数列表", "type": "array", "links": null},
          {"name": "原始消息", "type": "string", "links": null},
          {"name": "发送者ID", "type": "string", "links": null},
          {"name": "来源ID", "type": "string", "links": null}
        ],
        "properties": {"command": "time", "prefix": "/"},
        "boxcolor": "#3a3"
      },
      {
        "id": 6, "type": "action/send_message",
        "pos": [500, 600], "size": {"0": 210, "1": 78},
        "flags": {}, "order": 5, "mode": 0,
        "inputs": [
          {"name": "触发", "type": -1, "link": 3},
          {"name": "回复内容", "type": "string", "link": null}
        ],
        "outputs": [{"name": "完成", "type": -1, "links": null}],
        "properties": {"template": "⏰ 时间指令已触发"},
        "boxcolor": "#33a"
      },
      {
        "id": 7, "type": "event/on_command",
        "pos": [100, 850], "size": {"0": 210, "1": 182},
        "flags": {}, "order": 6, "mode": 0,
        "outputs": [
          {"name": "触发", "type": -1, "links": [4], "slot_index": 0},
          {"name": "指令名", "type": "string", "links": null},
          {"name": "参数列表", "type": "array", "links": null},
          {"name": "原始消息", "type": "string", "links": null},
          {"name": "发送者ID", "type": "string", "links": null},
          {"name": "来源ID", "type": "string", "links": null}
        ],
        "properties": {"command": "add", "prefix": "/"},
        "boxcolor": "#3a3"
      },
      {
        "id": 8, "type": "action/send_message",
        "pos": [500, 850], "size": {"0": 210, "1": 78},
        "flags": {}, "order": 7, "mode": 0,
        "inputs": [
          {"name": "触发", "type": -1, "link": 4},
          {"name": "回复内容", "type": "string", "link": null}
        ],
        "outputs": [{"name": "完成", "type": -1, "links": null}],
        "properties": {"template": "➕ 加法指令已触发"},
        "boxcolor": "#33a"
      }
    ],
    "links": [
      [1, 1, 0, 2, 0, -1],
      [2, 3, 0, 4, 0, -1],
      [3, 5, 0, 6, 0, -1],
      [4, 7, 0, 8, 0, -1]
    ],
    "groups": [], "config": {}, "extra": {}, "version": 0.4
  },
  "created_at": 1784265102,
  "updated_at": 1784265102
}`

// 测试用例
type TestCase struct {
	Msg      string
	Expected int // 预期回复数
}

// 测试结果
type TestResult struct {
	Command    string
	Success    bool
	ReplyCount int
	Duration   time.Duration
	Error      string
}

// HTTP 客户端（复用连接池）
var httpClient = &http.Client{
	Timeout: 60 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        2000,
		MaxIdleConnsPerHost: 2000,
		MaxConnsPerHost:     0, // 不限制
		IdleConnTimeout:     90 * time.Second,
	},
}

func main() {
	baseURL := flag.String("url", "http://127.0.0.1:4545", "主程序地址")
	username := flag.String("u", "admin", "登录用户名")
	password := flag.String("p", "admin", "登录密码")
	botID := flag.String("b", "sandbox", "机器人ID（沙盒模式用 sandbox）")
	concurrency := flag.Int("c", 10, "并发数")
	total := flag.Int("n", 100, "总请求数")
	cleanup := flag.Bool("cleanup", true, "测试完成后删除测试插件")
	flag.Parse()

	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║    ShizukuBot 并发指令解析测试（HTTP 端到端）    ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Printf("服务地址:    %s\n", *baseURL)
	fmt.Printf("并发数:      %d\n", *concurrency)
	fmt.Printf("总请求:      %d\n", *total)
	fmt.Printf("机器人ID:    %s\n\n", *botID)

	// 1. 登录获取 Cookie
	fmt.Println("🔐 正在登录...")
	cookie, err := login(*baseURL, *username, *password)
	if err != nil {
		fmt.Printf("❌ 登录失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 登录成功")

	// 2. 创建测试插件
	fmt.Println("📦 正在创建测试插件...")
	pluginID, err := createPlugin(*baseURL, cookie)
	if err != nil {
		fmt.Printf("❌ 创建插件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ 测试插件已创建: %s\n\n", pluginID)

	// 3. 等待插件加载
	time.Sleep(500 * time.Millisecond)

	// 4. 测试用例
	testCases := []TestCase{
		{"/ping", 1},
		{"/echo hello world", 1},
		{"/time", 1},
		{"/add 1 2", 1},
		{"/unknown", 0}, // 未注册指令
		{"/ping", 1},
		{"/echo 测试中文", 1},
	}

	totalRequests := *total
	if totalRequests < len(testCases) {
		totalRequests = len(testCases)
	}

	results := make([]TestResult, totalRequests)
	var successCount int64
	var failCount int64
	var totalDuration int64
	var idx int64

	startTime := time.Now()
	fmt.Printf("🚀 开始并发测试 (%d 并发, %d 请求)...\n\n", *concurrency, totalRequests)

	sem := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		go func(reqIdx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			curIdx := atomic.AddInt64(&idx, 1) - 1
			tc := testCases[reqIdx%len(testCases)]

			reqStart := time.Now()
			replies, err := sendSandboxMessage(*baseURL, cookie, *botID, tc.Msg, pluginID)
			duration := time.Since(reqStart)

			result := TestResult{
				Command:    tc.Msg,
				ReplyCount: len(replies),
				Duration:   duration,
			}

			if err != nil {
				result.Success = false
				result.Error = err.Error()
				atomic.AddInt64(&failCount, 1)
			} else if tc.Expected > 0 && len(replies) == 0 {
				result.Success = false
				result.Error = "期望有回复但实际无回复"
				atomic.AddInt64(&failCount, 1)
			} else if tc.Expected == 0 && len(replies) > 0 {
				result.Success = false
				result.Error = "期望无回复但实际有回复"
				atomic.AddInt64(&failCount, 1)
			} else {
				result.Success = true
				atomic.AddInt64(&successCount, 1)
			}

			atomic.AddInt64(&totalDuration, int64(duration))
			results[curIdx] = result
		}(i)
	}

	wg.Wait()
	totalElapsed := time.Since(startTime)

	// 输出统计报告
	fmt.Println("══════════════════════════════════════════════════")
	fmt.Println("📊 测试结果报告")
	fmt.Println("══════════════════════════════════════════════════")
	fmt.Printf("总请求数:    %d\n", totalRequests)
	fmt.Printf("成功:        %d ✅\n", atomic.LoadInt64(&successCount))
	fmt.Printf("失败:        %d ❌\n", atomic.LoadInt64(&failCount))
	fmt.Printf("总耗时:      %v\n", totalElapsed)
	if totalRequests > 0 {
		avgDuration := time.Duration(atomic.LoadInt64(&totalDuration) / int64(totalRequests))
		fmt.Printf("平均耗时:    %v\n", avgDuration)
		fmt.Printf("QPS:         %.2f\n", float64(totalRequests)/totalElapsed.Seconds())
	}
	fmt.Println()

	// 按指令分类统计
	cmdStats := make(map[string]*struct {
		count     int
		success   int
		totalTime time.Duration
	})
	for _, r := range results {
		if r.Command == "" {
			continue
		}
		s, ok := cmdStats[r.Command]
		if !ok {
			s = &struct {
				count     int
				success   int
				totalTime time.Duration
			}{}
			cmdStats[r.Command] = s
		}
		s.count++
		if r.Success {
			s.success++
		}
		s.totalTime += r.Duration
	}

	fmt.Println("📋 按指令统计:")
	fmt.Println("┌────────────────────┬──────┬──────┬───────────┐")
	fmt.Println("│ 指令               │ 次数 │ 成功 │ 平均耗时  │")
	fmt.Println("├────────────────────┼──────┼──────┼───────────┤")
	for cmd, s := range cmdStats {
		avg := time.Duration(0)
		if s.count > 0 {
			avg = s.totalTime / time.Duration(s.count)
		}
		fmt.Printf("│ %-18s │ %4d │ %4d │ %9v │\n", cmd, s.count, s.success, avg)
	}
	fmt.Println("└────────────────────┴──────┴──────┴───────────┘")
	fmt.Println()

	// 输出失败详情
	failCountInt := int(atomic.LoadInt64(&failCount))
	if failCountInt > 0 {
		fmt.Println("❌ 失败请求详情 (最多显示10条):")
		shown := 0
		for i, r := range results {
			if !r.Success && shown < 10 {
				fmt.Printf("  #%d 指令=%s 耗时=%v 错误=%s\n", i, r.Command, r.Duration, r.Error)
				shown++
			}
		}
		if failCountInt > 10 {
			fmt.Printf("  ... 还有 %d 条失败请求未显示\n", failCountInt-10)
		}
		fmt.Println()
	}

	// 清理
	if *cleanup {
		fmt.Println("🧹 正在清理测试插件...")
		if err := deletePlugin(*baseURL, cookie, pluginID); err != nil {
			fmt.Printf("⚠️ 清理失败: %v\n", err)
		} else {
			fmt.Println("✅ 已清理测试插件")
		}
	}

	if failCountInt > 0 {
		os.Exit(1)
	}
	fmt.Println("✨ 所有测试通过！")
}

// login 登录获取 session cookie
func login(baseURL, username, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	resp, err := httpClient.Post(baseURL+"/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("登录失败: HTTP %d", resp.StatusCode)
	}

	// 提取 Set-Cookie（gorilla/sessions 默认 cookie 名为 shizukubot_session）
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "shizukubot_session" {
			return "shizukubot_session=" + cookie.Value, nil
		}
	}
	return "", fmt.Errorf("未获取到 session cookie")
}

// createPlugin 通过 API 创建测试插件
func createPlugin(baseURL, cookie string) (string, error) {
	var pluginData map[string]interface{}
	if err := json.Unmarshal([]byte(testPluginJSON), &pluginData); err != nil {
		return "", err
	}

	createBody, _ := json.Marshal(map[string]interface{}{
		"name":        "并发测试插件",
		"description": "用于并发指令解析测试",
		"bot_id":      "*",
		"graph":       pluginData["graph"],
	})

	req, _ := http.NewRequest("POST", baseURL+"/api/plugins", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("创建失败: code=%d", result.Code)
	}
	if result.Data.ID == "" {
		return "", fmt.Errorf("返回的插件ID为空")
	}

	// 启用插件
	toggleURL := fmt.Sprintf("%s/api/plugins/%s/toggle", baseURL, result.Data.ID)
	req2, _ := http.NewRequest("POST", toggleURL, nil)
	req2.Header.Set("Cookie", cookie)
	resp2, err := httpClient.Do(req2)
	if err != nil {
		return result.Data.ID, nil
	}
	defer resp2.Body.Close()

	return result.Data.ID, nil
}

// sendSandboxMessage 通过沙盒 API 发送消息
func sendSandboxMessage(baseURL, cookie, botID, content, pluginID string) ([]string, error) {
	body, _ := json.Marshal(map[string]string{
		"type":      "c2c",
		"content":   content,
		"plugin_id": pluginID,
	})

	url := fmt.Sprintf("%s/api/bots/%s/sandbox/send", baseURL, botID)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Replies []string `json:"replies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Replies, nil
}

// deletePlugin 删除测试插件
func deletePlugin(baseURL, cookie, pluginID string) error {
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/plugins/%s", baseURL, pluginID), nil)
	req.Header.Set("Cookie", cookie)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
