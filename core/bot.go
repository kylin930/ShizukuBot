package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kylin930/botgo-plus"
	"github.com/kylin930/botgo-plus/openapi"
	"github.com/kylin930/botgo-plus/token"
	"golang.org/x/oauth2"

	"shizukubot/config"
	"shizukubot/events"
)

type Bot struct {
	ID            string
	Name          string
	AppID         string
	AppSecret     string `json:"-"`
	Mode          string
	OpenAPI       openapi.OpenAPI
	TokenSource   oauth2.TokenSource
	diagnostics   QQDiagnostics
	Enabled       bool
	Running       bool
	Stats         *Stats
	LogManager    *LogManager
	Dispatcher    *events.Dispatcher
	Commands      func([]string, string) []string
	lifecycle     sync.Mutex
	workers       sync.WaitGroup
	PluginIDs     []string
	CommandPrefix string
	AdminUsers    []string
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.Mutex
}

func NewBot(cfg config.BotConfig) (*Bot, error) {
	if cfg.ID == "" || cfg.AppID == "" || cfg.AppSecret == "" || (cfg.Mode != "" && cfg.Mode != "websocket" && cfg.Mode != "webhook") {
		return nil, fmt.Errorf("bot config is invalid")
	}

	credentials := &token.QQBotCredentials{
		AppID:     cfg.AppID,
		AppSecret: cfg.AppSecret,
	}

	tokenSource := token.NewQQBotTokenSource(credentials)

	api := botgo.NewOpenAPI(cfg.AppID, tokenSource).WithTimeout(5 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	mode := cfg.Mode
	if mode == "" {
		mode = "webhook"
	}

	prefix := cfg.CommandPrefix
	if prefix == "" {
		prefix = "/"
	}

	return &Bot{
		ID:            cfg.ID,
		Name:          cfg.Name,
		AppID:         cfg.AppID,
		AppSecret:     cfg.AppSecret,
		Mode:          mode,
		OpenAPI:       api,
		TokenSource:   tokenSource,
		Enabled:       cfg.Enabled,
		Stats:         NewStats(),
		PluginIDs:     cfg.Plugins,
		CommandPrefix: prefix,
		AdminUsers:    cfg.AdminUsers,
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

func (b *Bot) SetLogManager(lm *LogManager) {
	b.LogManager = lm
}

func (b *Bot) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Running
}

func (b *Bot) logDebug(format string, args ...interface{}) {
	if b.LogManager != nil {
		b.LogManager.Debug(b.ID, fmt.Sprintf(format, args...))
	}
}

func (b *Bot) logInfo(format string, args ...interface{}) {
	if b.LogManager != nil {
		b.LogManager.Info(b.ID, fmt.Sprintf(format, args...))
	}
}

func (b *Bot) logWarn(format string, args ...interface{}) {
	if b.LogManager != nil {
		b.LogManager.Warn(b.ID, fmt.Sprintf(format, args...))
	}
}

func (b *Bot) logError(format string, args ...interface{}) {
	if b.LogManager != nil {
		b.LogManager.Error(b.ID, fmt.Sprintf(format, args...))
	}
}

// IsEnabled returns the configured startup flag under the bot lock.
func (b *Bot) IsEnabled() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.Enabled }

// ConfigSnapshot copies mutable bot settings without exposing shared slices.
func (b *Bot) ConfigSnapshot() config.BotConfig {
	b.mu.Lock()
	defer b.mu.Unlock()
	return config.BotConfig{ID: b.ID, Name: b.Name, AppID: b.AppID, AppSecret: b.AppSecret, Mode: b.Mode, Enabled: b.Enabled, Plugins: append([]string(nil), b.PluginIDs...), CommandPrefix: b.CommandPrefix, AdminUsers: append([]string(nil), b.AdminUsers...)}
}

func (b *Bot) requestContext() context.Context {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ctx != nil {
		return b.ctx
	}
	return context.Background()
}
