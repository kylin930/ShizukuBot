package config

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
)

type WebUIConfig struct {
	Enabled  bool   `json:"enabled"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	Host     string `json:"host"`
}

type BotConfig struct {
	ID            string   `json:"id"`
	AppID         string   `json:"app_id"`
	AppSecret     string   `json:"app_secret"`
	Name          string   `json:"name"`
	Enabled       bool     `json:"enabled"`
	Mode          string   `json:"mode"`
	Plugins       []string `json:"plugins"`        // 装载的插件ID列表，为空则装载所有启用插件
	CommandPrefix string   `json:"command_prefix"` // 指令前缀，为空则默认 "/"
	AdminUsers    []string `json:"admin_users"`    // 管理员用户ID列表
}

// BuiltinCommandsConfig 内置指令的回复模板（全局一份）
// 支持占位符替换：{bot名称} 等；/help 支持 {指令列表}，#shizuku 支持 {core版本}/{运行时间}/{操作系统}/{go版本}
type BuiltinCommandsConfig struct {
	Help    string `json:"help"`    // /help 回复模板
	Status  string `json:"status"`  // /status 回复模板
	Shizuku string `json:"shizuku"` // #shizuku 回复模板
}

type Config struct {
	WebUI           WebUIConfig         `json:"webui"`
	Bots            []BotConfig         `json:"bots"`
	MarketAPI       string              `json:"market_api"`
	BuiltinCommands BuiltinCommandsConfig `json:"builtin_commands"`
}

func LoadConfig(filePath string) (*Config, error) {
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		config := createDefaultConfig()
		if err := SaveConfig(filePath, config); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
		return config, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to check config file: %w", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if config.WebUI.Password == "" {
		config.WebUI.Password = generateRandomPassword()
		if err := SaveConfig(filePath, &config); err != nil {
			return nil, fmt.Errorf("failed to save config with generated password: %w", err)
		}
	}

	// 兼容旧配置：内置指令模板缺失时补默认值
	defaults := defaultBuiltinCommands()
	needSave := false
	if config.BuiltinCommands.Help == "" {
		config.BuiltinCommands.Help = defaults.Help
		needSave = true
	}
	if config.BuiltinCommands.Status == "" {
		config.BuiltinCommands.Status = defaults.Status
		needSave = true
	}
	if config.BuiltinCommands.Shizuku == "" {
		config.BuiltinCommands.Shizuku = defaults.Shizuku
		needSave = true
	}
	if needSave {
		if err := SaveConfig(filePath, &config); err != nil {
			return nil, fmt.Errorf("failed to save config with builtin command defaults: %w", err)
		}
	}

	return &config, nil
}

// defaultBuiltinCommands 返回内置指令的默认回复模板
func defaultBuiltinCommands() BuiltinCommandsConfig {
	return BuiltinCommandsConfig{
		Help:    "可用指令列表：\n{指令列表}\n\n输入 /指令名 查看更详细的信息",
		Status:  "{bot名称} 运行中\n收到消息：{接收消息数}\n发送消息：{发送消息数}\nCPU：{CPU占用}\n内存：{内存占用}",
		Shizuku: "ShizukuBot 框架信息\n版本：v{core版本}\n运行时间：{运行时间}\n系统：{操作系统}\nGo版本：{go版本}",
	}
}

func SaveConfig(filePath string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func createDefaultConfig() *Config {
	return &Config{
		WebUI: WebUIConfig{
			Enabled:  true,
			Port:     4545,
			Password: generateRandomPassword(),
		},
		Bots: []BotConfig{
			{
				ID:         "bot1",
				AppID:      "",
				AppSecret:  "",
				Name:       "ShizukuBot",
				Enabled:    false,
				Mode:       "websocket",
			},
		},
		MarketAPI:       "https://api.nekopara.us/market/api",
		BuiltinCommands: defaultBuiltinCommands(),
	}
}

func generateRandomPassword() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	rand.Read(b)
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}