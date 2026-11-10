package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"shizukubot/selfupdate"
)

const (
	rebuildTimeout  = 10 * time.Minute
	rebuildLogLimit = 32 * 1024
	rebuildSettle   = 1200 * time.Millisecond
)

// rebuildState 记录一次编译任务的进度，供前端轮询。编译耗时较长，
// 不能同步等待 HTTP 响应（服务端 WriteTimeout 为 15 秒）。
type rebuildState struct {
	mu            sync.Mutex
	running       bool
	success       bool
	output        string
	errText       string
	restartQueued bool
	startedAt     time.Time
	finishedAt    time.Time
}

func (s *rebuildState) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{
		"running":        s.running,
		"success":        s.success,
		"output":         s.output,
		"error":          s.errText,
		"restart_queued": s.restartQueued,
	}
	if !s.startedAt.IsZero() {
		out["started_at"] = s.startedAt
	}
	if !s.finishedAt.IsZero() {
		out["finished_at"] = s.finishedAt
	}
	return out
}

// StartPluginRebuild 编译当前源码树并生成新的 shizukubot 二进制。
// restart 为真时，编译成功后自动重启主程序。
func (r *Router) StartPluginRebuild(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Restart *bool `json:"restart"`
	}
	if req.Body != nil {
		_ = json.NewDecoder(io.LimitReader(req.Body, 1<<16)).Decode(&body)
	}
	restart := body.Restart == nil || *body.Restart

	root, executable, err := rebuildPaths()
	if err != nil {
		BadRequestResponse(w, err.Error())
		return
	}
	staged := selfupdate.Staged(executable)

	r.rebuild.mu.Lock()
	if r.rebuild.running {
		r.rebuild.mu.Unlock()
		BadRequestResponse(w, "已有编译任务在执行，请等待完成")
		return
	}
	r.rebuild.running = true
	r.rebuild.success = false
	r.rebuild.output = ""
	r.rebuild.errText = ""
	r.rebuild.restartQueued = false
	r.rebuild.startedAt = time.Now()
	r.rebuild.finishedAt = time.Time{}
	r.rebuild.mu.Unlock()

	go func() {
		output, buildErr := compilePlugins(root, staged)
		queued := buildErr == nil && restart && r.OnReloadRequest != nil

		r.rebuild.mu.Lock()
		r.rebuild.running = false
		r.rebuild.success = buildErr == nil
		r.rebuild.output = output
		if buildErr != nil {
			r.rebuild.errText = buildErr.Error()
		}
		r.rebuild.restartQueued = queued
		r.rebuild.finishedAt = time.Now()
		r.rebuild.mu.Unlock()

		if !queued {
			return
		}
		// 等前端拿到任务结果并写回响应后再停服替换。
		time.Sleep(rebuildSettle)
		r.OnReloadRequest()
	}()

	SuccessResponse(w, map[string]any{"started": true, "staged": staged})
}

// PluginRebuildStatus 返回最近一次编译任务的状态与日志。
func (r *Router) PluginRebuildStatus(w http.ResponseWriter, req *http.Request) {
	SuccessResponse(w, r.rebuild.snapshot())
}

// rebuildPaths 定位源码根目录（含 go.mod）与主程序路径。
func rebuildPaths() (string, string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", "", errors.New("无法定位主程序路径")
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", "", errors.New("无法定位主程序路径")
	}
	candidates := []string{}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	candidates = append(candidates, filepath.Dir(executable))
	for _, dir := range candidates {
		if info, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil && !info.IsDir() {
			return dir, executable, nil
		}
	}
	return "", "", errors.New("未找到插件源码（go.mod），当前目录不支持在线编译")
}

// compilePlugins 先重建插件注册表，再编译新的主程序二进制。
func compilePlugins(root, staged string) (string, error) {
	if _, err := exec.LookPath("go"); err != nil {
		return "", errors.New("未找到 Go 工具链，请确认 go 命令在 PATH 中")
	}
	ctx, cancel := context.WithTimeout(context.Background(), rebuildTimeout)
	defer cancel()

	var log bytes.Buffer
	run := func(args ...string) error {
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		log.Write(output)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("编译超时（%s）", rebuildTimeout)
		}
		return fmt.Errorf("go %s 失败：%v", strings.Join(args, " "), err)
	}
	if err := run("generate", "./pluginregistry"); err != nil {
		return tail(log.Bytes()), err
	}
	if err := run("build", "-trimpath", "-o", staged, "."); err != nil {
		_ = os.Remove(staged)
		return tail(log.Bytes()), err
	}
	if !selfupdate.Exists(staged) {
		return tail(log.Bytes()), errors.New("构建产物为空")
	}
	return tail(log.Bytes()), nil
}

// tail 只保留日志尾部，避免长时间构建的输出撑爆响应。
func tail(data []byte) string {
	if len(data) > rebuildLogLimit {
		data = data[len(data)-rebuildLogLimit:]
	}
	return string(data)
}
