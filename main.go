package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"shizukubot/migration"
	"syscall"
	"time"

	"shizukubot/api"
	"shizukubot/config"
	"shizukubot/core"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "migrate-legacy" {
		if err := migrateCLI(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// 注册常见 MIME 类型，防止 Windows 注册表缺少映射
	mime.AddExtensionType(".css", "text/css; charset=utf-8")
	mime.AddExtensionType(".js", "application/javascript; charset=utf-8")
	mime.AddExtensionType(".json", "application/json; charset=utf-8")
	mime.AddExtensionType(".html", "text/html; charset=utf-8")
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".woff", "font/woff")
	mime.AddExtensionType(".woff2", "font/woff2")

	logInit := func(msg string) {
		fmt.Println(msg)
		if core.GlobalLogManager != nil {
			core.GlobalLogManager.Info("", msg)
		}
	}

	logInit("========================================")
	logInit("     ShizukuBot v" + core.CoreVersion)
	logInit("========================================")

	cfgPath := "config/config.json"
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Printf("❌ Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 注入内置指令回复模板，供运行中的 bot 读取（配置页保存时会热更新）
	core.SetCommandTemplates(&core.CommandTemplates{
		Help:    cfg.BuiltinCommands.Help,
		Status:  cfg.BuiltinCommands.Status,
		Shizuku: cfg.BuiltinCommands.Shizuku,
	})

	botManager := core.NewBotManager()
	logManager := botManager.GetLogManager()

	for _, botCfg := range cfg.Bots {
		bot, err := core.NewBot(botCfg)
		if err != nil {
			fmt.Printf("❌ Failed to create bot %s: %v\n", botCfg.ID, err)
			continue
		}

		if err := botManager.AddBot(bot); err != nil {
			fmt.Printf("❌ Failed to add bot %s: %v\n", botCfg.ID, err)
			continue
		}

		status := "❌ Disabled"
		if bot.Enabled {
			status = "✅ Enabled"
		}
		fmt.Printf("   Bot [%s] %s - %s\n", bot.ID, bot.Name, status)
	}

	router := api.NewRouter(botManager, cfg, cfgPath, logManager)

	if err := botManager.StartAll(); err != nil {
		fmt.Printf("⚠️  Some bots failed to start: %v\n", err)
	}

	fmt.Println("✅ All bots started")

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.WebUI.Port),
		Handler:      router.WithCORS(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		fmt.Printf("🚀 WebUI Server listening on http://localhost:%d\n", cfg.WebUI.Port)
		fmt.Printf("📡 Webhook Endpoint: http://localhost:%d/webhook/{botID}\n", cfg.WebUI.Port)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("❌ Server failed to start: %v\n", err)
			os.Exit(1)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	fmt.Printf("\n📢 Received signal: %s, shutting down gracefully...\n", sig)

	botManager.StopAll()
	pluginCtx, pluginCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := router.Close(pluginCtx); err != nil {
		fmt.Println("plugin shutdown failed")
	}
	pluginCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("❌ Server shutdown failed: %v\n", err)
		os.Exit(1)
	}

	if err := logManager.SaveLogsToFile(); err != nil {
		fmt.Printf("❌ Failed to save logs: %v\n", err)
	} else {
		fmt.Println("✅ Logs saved to logs/ directory")
	}

	fmt.Println("✅ ShizukuBot has been stopped")
}

func migrateCLI(args []string) error {
	flags := flag.NewFlagSet("migrate-legacy", flag.ContinueOnError)
	apply := flags.Bool("apply", false, "back up and generate disabled code plugins")
	dry := flags.Bool("dry-run", false, "detect only; default")
	rollback := flags.String("rollback", "", "rollback an unmodified migration backup (stop host first)")
	source := flags.String("source", "data/plugins", "legacy directory")
	custom := flags.String("custom-nodes", "data/custom_nodes", "legacy custom node directory")
	destination := flags.String("destination", "data/code-plugins", "new plugin directory")
	backups := flags.String("backups", "data/legacy-backups", "verified backup directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *apply && *dry {
		return fmt.Errorf("choose apply or dry-run")
	}
	if *rollback != "" {
		return migration.Rollback(*rollback, *destination)
	}
	report, err := migration.Run(migration.Options{Source: *source, CustomNodes: *custom, Destination: *destination, Backups: *backups, Apply: *apply})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}
