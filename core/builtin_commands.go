package core

import (
	"fmt"
	"strconv"
	"strings"

	"context"
	"shizukubot/config"
	"shizukubot/events"
	"sync"
)

// CommandTemplates 内置指令的回复模板（全局一份，由 main 注入，配置页可热更新）
type CommandTemplates struct {
	DisableHelp       bool
	DisableStatus     bool
	HideShizukuInHelp bool
	Help              string
	Status            string
	Shizuku           string
}

// GlobalCommandTemplates 全局内置指令模板，仿 GlobalLogManager 模式。
// 由 main.go 在加载配置后注入，api/config.go 在 UpdateConfig 时同步更新。
var templateMu sync.RWMutex
var globalCommandTemplates *CommandTemplates

func SetCommandTemplates(t *CommandTemplates) {
	templateMu.Lock()
	defer templateMu.Unlock()
	if t == nil {
		globalCommandTemplates = nil
		return
	}
	snapshot := *t
	globalCommandTemplates = &snapshot
}

// handleBuiltinCommand 检查并处理内置指令（/help、/status、#shizuku）。
// 命中则回复并返回 true（调用方应跳过插件图执行）；未命中返回 false。
// 匹配规则与 event/on_command 一致：按 CommandPrefix 切分，首词精确匹配指令名。
func (b *Bot) HandleBuiltinCommand(ctx context.Context, ev events.Event) bool {
	b.mu.Lock()
	// Only snapshot under the bot lock. Providers, system probes and replies may block.
	view := &Bot{ID: b.ID, Name: b.Name, CommandPrefix: b.CommandPrefix, PluginIDs: append([]string(nil), b.PluginIDs...), Stats: b.Stats, Commands: b.Commands, LogManager: b.LogManager}
	b.mu.Unlock()
	return view.handleBuiltinSnapshot(ctx, ev)
}

func (b *Bot) handleBuiltinSnapshot(ctx context.Context, ev events.Event) bool {
	msg := strings.TrimSpace(ev.Message)
	if msg == "" {
		return false
	}

	prefix := b.CommandPrefix
	if prefix == "" {
		prefix = "/"
	}

	// #shizuku 使用固定 # 前缀，与 CommandPrefix 无关
	if strings.HasPrefix(msg, "#") {
		parts := strings.Fields(msg)
		if len(parts) > 0 && parts[0] == "#shizuku" {
			b.replyBuiltin(ctx, ev, b.renderShizuku())
			return true
		}
	}

	// /help、/status 使用 bot 的 CommandPrefix
	if strings.HasPrefix(msg, prefix) {
		parts := strings.Fields(strings.TrimPrefix(msg, prefix))
		if len(parts) == 0 {
			return false
		}
		switch parts[0] {
		case "help":
			if currentTemplates().DisableHelp {
				return false
			}
			b.replyBuiltin(ctx, ev, b.renderHelp())
			return true
		case "status":
			if currentTemplates().DisableStatus {
				return false
			}
			b.replyBuiltin(ctx, ev, b.renderStatus())
			return true
		}
	}

	return false
}

// replyBuiltin 按消息来源（群/私聊）选择回复通道
func (b *Bot) replyBuiltin(ctx context.Context, ev events.Event, content string) {
	if ev.Reply != nil && content != "" {
		if err := ev.Reply(ctx, events.Reply{Kind: "text", Text: content}); err != nil {
			b.logError("builtin reply failed")
		}
	}
}

// renderHelp 渲染 /help 回复
func (b *Bot) renderHelp() string {
	tmpl := defaultIfEmpty(currentTemplates().Help, "📋 可用指令列表：\n{指令列表}")
	vars := map[string]interface{}{
		"指令列表":  b.collectCommandList(),
		"bot名称": b.Name,
	}
	return renderTemplate(tmpl, vars)
}

// renderStatus 渲染 /status 回复
func (b *Bot) renderStatus() string {
	stats := b.Stats.Snapshot()
	tmpl := defaultIfEmpty(currentTemplates().Status, "🤖 {bot名称} 运行中")
	vars := map[string]interface{}{
		"bot名称": b.Name,
		"接收消息数": strconv.FormatInt(stats.ReceivedMessages, 10),
		"发送消息数": strconv.FormatInt(stats.SentMessages, 10),
		"CPU占用": CoreCPUPercent(),
		"内存占用":  CoreProcMemRSS(),
	}
	return renderTemplate(tmpl, vars)
}

// renderShizuku 渲染 #shizuku 回复
func (b *Bot) renderShizuku() string {
	tmpl := defaultIfEmpty(currentTemplates().Shizuku, "🌸 ShizukuBot 框架信息\n版本：v{core版本}")
	vars := map[string]interface{}{
		"core版本": CoreVersion,
		"运行时间":   CoreUptime(),
		"操作系统":   CoreOS(),
		"go版本":   GoVersion(),
		"bot名称":  b.Name,
	}
	return renderTemplate(tmpl, vars)
}

// collectCommandList combines enabled builtins with registered plugin commands.
func (b *Bot) collectCommandList() string {
	settings := currentTemplates()
	prefix := b.CommandPrefix
	if prefix == "" {
		prefix = "/"
	}
	all := []string{}
	if !settings.DisableHelp {
		all = append(all, prefix+"help")
	}
	if !settings.DisableStatus {
		all = append(all, prefix+"status")
	}
	if !settings.HideShizukuInHelp {
		all = append(all, "#shizuku")
	}
	if b.Commands != nil {
		all = append(all, b.Commands(b.PluginIDs, b.ID)...)
	}
	seen := map[string]bool{}
	visible := []string{}
	for _, command := range all {
		if command == "" || seen[command] || (settings.HideShizukuInHelp && command == "#shizuku") {
			continue
		}
		seen[command] = true
		visible = append(visible, command)
	}
	return strings.Join(visible, "\n")
}

func ConfigureBuiltinCommands(c config.BuiltinCommandsConfig) {
	SetCommandTemplates(&CommandTemplates{Help: c.Help, Status: c.Status, Shizuku: c.Shizuku,
		DisableHelp: !config.BuiltinEnabled(c.HelpEnabled), DisableStatus: !config.BuiltinEnabled(c.StatusEnabled), HideShizukuInHelp: !config.BuiltinEnabled(c.ShowShizukuInHelp)})
}

// currentTemplates 安全获取全局模板（未注入时返回零值）
func currentTemplates() CommandTemplates {
	templateMu.RLock()
	defer templateMu.RUnlock()
	if globalCommandTemplates != nil {
		return *globalCommandTemplates
	}
	return CommandTemplates{}
}

// renderTemplate 将模板中的 {key} 替换为对应值（与 TemplateExecutor 逻辑一致）
func renderTemplate(template string, vars map[string]interface{}) string {
	result := template
	for k, v := range vars {
		placeholder := "{" + k + "}"
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", v))
	}
	return result
}

// defaultIfEmpty 模板为空时用 fallback
func defaultIfEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// MatchesBuiltin reserves existing core commands before plugin routing. It performs no I/O.
func (b *Bot) MatchesBuiltin(e events.Event) bool {
	cfg := b.ConfigSnapshot()
	message := strings.TrimSpace(e.Message)
	parts := strings.Fields(message)
	if len(parts) > 0 && parts[0] == "#shizuku" {
		return true
	}
	prefix := cfg.CommandPrefix
	if prefix == "" {
		prefix = "/"
	}
	if !strings.HasPrefix(message, prefix) {
		return false
	}
	parts = strings.Fields(strings.TrimPrefix(message, prefix))
	if len(parts) == 0 {
		return false
	}
	settings := currentTemplates()
	return (parts[0] == "help" && !settings.DisableHelp) || (parts[0] == "status" && !settings.DisableStatus)
}
