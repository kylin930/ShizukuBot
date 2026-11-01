package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/tencent-connect/botgo"
	"github.com/tencent-connect/botgo/constant"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/dto/keyboard"
	"github.com/tencent-connect/botgo/openapi"
	"github.com/tencent-connect/botgo/token"
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
	restyClient   *resty.Client
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
		mode = "websocket"
	}

	prefix := cfg.CommandPrefix
	if prefix == "" {
		prefix = "/"
	}

	restyClient := resty.New()
	restyClient.SetTimeout(5 * time.Second)

	return &Bot{
		ID:            cfg.ID,
		Name:          cfg.Name,
		AppID:         cfg.AppID,
		AppSecret:     cfg.AppSecret,
		Mode:          mode,
		OpenAPI:       api,
		TokenSource:   tokenSource,
		restyClient:   restyClient,
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

func (b *Bot) SendGroupMessage(groupID, content string) error {
	msg := &dto.MessageToCreate{
		Content: content,
	}

	if _, err := b.OpenAPI.PostGroupMessage(context.Background(), groupID, msg); err != nil {
		b.logError("failed to send group message: %v", err)
		return err
	}

	b.Stats.IncSent()
	b.logInfo("sent group message to %s: %s", groupID, content)
	return nil
}

func (b *Bot) SendC2CMessage(userID, content string) error {
	msg := &dto.MessageToCreate{
		Content: content,
	}

	if _, err := b.OpenAPI.PostC2CMessage(context.Background(), userID, msg); err != nil {
		b.logError("failed to send C2C message: %v", err)
		return err
	}

	b.Stats.IncSent()
	b.logInfo("sent C2C message to %s: %s", userID, content)
	return nil
}

// parseDataURL 解析 "data:image/<type>;base64,XXXX" 形式的 Data URL。
// 成功时返回去掉 "data:...;base64," 头后的 base64 字符串（与腾讯 file_data 要求一致）；
// 若 imageURL 不是 data:image 形式或 base64 非法，则返回 ""。
func parseDataURL(imageURL string) string {
	if !strings.HasPrefix(imageURL, "data:image") {
		return ""
	}
	comma := strings.Index(imageURL, ",")
	if comma < 0 {
		return ""
	}
	b64 := imageURL[comma+1:]
	if _, err := base64.StdEncoding.DecodeString(b64); err != nil {
		return ""
	}
	return b64
}

func (b *Bot) SendGroupImage(groupID, imageURL string) error {
	// data: URL 走 base64 直传（无需公网），http(s) URL 走腾讯回源
	if b64 := parseDataURL(imageURL); b64 != "" {
		return b.sendGroupImageDataViaSDK(groupID, b64)
	}

	uploadMsg := &dto.RichMediaMessage{
		FileType:   1,
		URL:        imageURL,
		SrvSendMsg: false,
	}
	resp, err := b.OpenAPI.PostGroupMessage(context.Background(), groupID, uploadMsg)
	if err != nil {
		b.logError("failed to upload group image: %v", err)
		return err
	}

	msg := &dto.MessageToCreate{
		MsgType: dto.RichMediaMsg,
		Media:   &dto.MediaInfo{FileInfo: resp.FileInfo},
	}
	if _, err := b.OpenAPI.PostGroupMessage(context.Background(), groupID, msg); err != nil {
		b.logError("failed to send group image: %v", err)
		return err
	}

	b.Stats.IncSent()
	b.logInfo("sent group image to %s: %s", groupID, imageURL)
	return nil
}

// sendGroupImageDataViaSDK 用打补丁后的 RichMediaMessage.FileData 直传 base64 到 /v2/groups/{id}/files，
// 拿到 file_info 后再以 msg_type=7 发送图片消息。替代了原先手写 resty multipart 的 SendGroupImageData。
func (b *Bot) sendGroupImageDataViaSDK(groupID, base64NoHeader string) error {
	uploadMsg := &dto.RichMediaMessage{
		FileType:   1,
		FileData:   base64NoHeader,
		SrvSendMsg: false,
	}
	resp, err := b.OpenAPI.PostGroupMessage(context.Background(), groupID, uploadMsg)
	if err != nil {
		b.logError("failed to upload group image (base64): %v", err)
		return err
	}

	msg := &dto.MessageToCreate{
		MsgType: dto.RichMediaMsg,
		Media:   &dto.MediaInfo{FileInfo: resp.FileInfo},
	}
	if _, err := b.OpenAPI.PostGroupMessage(context.Background(), groupID, msg); err != nil {
		b.logError("failed to send group image (base64): %v", err)
		return err
	}

	b.Stats.IncSent()
	b.logInfo("sent group image data to %s", groupID)
	return nil
}

func (b *Bot) SendGroupImageData(groupID string, imageData []byte) error {
	token, err := b.TokenSource.Token()
	if err != nil {
		b.logError("failed to get access token: %v", err)
		return err
	}

	domain := constant.APIDomain
	url := fmt.Sprintf("%s/v2/groups/%s/files", domain, groupID)

	resp, err := b.restyClient.R().
		SetHeader("Authorization", "QQBot "+token.AccessToken).
		SetHeader("Content-Type", "multipart/form-data").
		SetMultipartFormData(map[string]string{
			"file_type":    "1",
			"srv_send_msg": "false",
		}).
		SetFileReader("file", "image.png", bytes.NewReader(imageData)).
		Post(url)

	if err != nil {
		b.logError("failed to upload group image data: %v", err)
		return err
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			FileInfo []byte `json:"file_info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		b.logError("failed to parse upload response: %v", err)
		return err
	}

	if result.Code != 0 {
		b.logError("upload failed: code=%d", result.Code)
		return fmt.Errorf("upload failed: code=%d", result.Code)
	}

	msg := &dto.MessageToCreate{
		MsgType: dto.RichMediaMsg,
		Media:   &dto.MediaInfo{FileInfo: result.Data.FileInfo},
	}
	if _, err := b.OpenAPI.PostGroupMessage(context.Background(), groupID, msg); err != nil {
		b.logError("failed to send group image: %v", err)
		return err
	}

	b.Stats.IncSent()
	b.logInfo("sent group image data to %s", groupID)
	return nil
}

func (b *Bot) SendC2CImage(userID, imageURL string) error {
	// data: URL 走 base64 直传（无需公网），http(s) URL 走腾讯回源
	if b64 := parseDataURL(imageURL); b64 != "" {
		return b.sendC2CImageDataViaSDK(userID, b64)
	}

	uploadMsg := &dto.RichMediaMessage{
		FileType:   1,
		URL:        imageURL,
		SrvSendMsg: false,
	}
	resp, err := b.OpenAPI.PostC2CMessage(context.Background(), userID, uploadMsg)
	if err != nil {
		b.logError("failed to upload C2C image: %v", err)
		return err
	}

	msg := &dto.MessageToCreate{
		MsgType: dto.RichMediaMsg,
		Media:   &dto.MediaInfo{FileInfo: resp.FileInfo},
	}
	if _, err := b.OpenAPI.PostC2CMessage(context.Background(), userID, msg); err != nil {
		b.logError("failed to send C2C image: %v", err)
		return err
	}

	b.Stats.IncSent()
	b.logInfo("sent C2C image to %s: %s", userID, imageURL)
	return nil
}

// sendC2CImageDataViaSDK 用打补丁后的 RichMediaMessage.FileData 直传 base64 到 /v2/users/{id}/files，
// 拿到 file_info 后再以 msg_type=7 发送图片消息。替代了原先手写 resty multipart 的 SendC2CImageData。
func (b *Bot) sendC2CImageDataViaSDK(userID, base64NoHeader string) error {
	uploadMsg := &dto.RichMediaMessage{
		FileType:   1,
		FileData:   base64NoHeader,
		SrvSendMsg: false,
	}
	resp, err := b.OpenAPI.PostC2CMessage(context.Background(), userID, uploadMsg)
	if err != nil {
		b.logError("failed to upload C2C image (base64): %v", err)
		return err
	}

	msg := &dto.MessageToCreate{
		MsgType: dto.RichMediaMsg,
		Media:   &dto.MediaInfo{FileInfo: resp.FileInfo},
	}
	if _, err := b.OpenAPI.PostC2CMessage(context.Background(), userID, msg); err != nil {
		b.logError("failed to send C2C image (base64): %v", err)
		return err
	}

	b.Stats.IncSent()
	b.logInfo("sent C2C image data to %s", userID)
	return nil
}

func (b *Bot) SendC2CImageData(userID string, imageData []byte) error {
	token, err := b.TokenSource.Token()
	if err != nil {
		b.logError("failed to get access token: %v", err)
		return err
	}

	domain := constant.APIDomain
	url := fmt.Sprintf("%s/v2/users/%s/files", domain, userID)

	resp, err := b.restyClient.R().
		SetHeader("Authorization", "QQBot "+token.AccessToken).
		SetHeader("Content-Type", "multipart/form-data").
		SetMultipartFormData(map[string]string{
			"file_type":    "1",
			"srv_send_msg": "false",
		}).
		SetFileReader("file", "image.png", bytes.NewReader(imageData)).
		Post(url)

	if err != nil {
		b.logError("failed to upload C2C image data: %v", err)
		return err
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			FileInfo []byte `json:"file_info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		b.logError("failed to parse upload response: %v", err)
		return err
	}

	if result.Code != 0 {
		b.logError("upload failed: code=%d", result.Code)
		return fmt.Errorf("upload failed: code=%d", result.Code)
	}

	msg := &dto.MessageToCreate{
		MsgType: dto.RichMediaMsg,
		Media:   &dto.MediaInfo{FileInfo: result.Data.FileInfo},
	}
	if _, err := b.OpenAPI.PostC2CMessage(context.Background(), userID, msg); err != nil {
		b.logError("failed to send C2C image: %v", err)
		return err
	}

	b.Stats.IncSent()
	b.logInfo("sent C2C image data to %s", userID)
	return nil
}

func (b *Bot) SendGroupRichMedia(groupID, text, imageURL string) error {
	// 先上传图片获取 file_info。data: URL 走 base64 直传（无需公网），http(s) URL 走腾讯回源。
	var uploadMsg *dto.RichMediaMessage
	if b64 := parseDataURL(imageURL); b64 != "" {
		uploadMsg = &dto.RichMediaMessage{
			FileType:   1,
			FileData:   b64,
			SrvSendMsg: false,
		}
	} else {
		uploadMsg = &dto.RichMediaMessage{
			FileType:   1,
			URL:        imageURL,
			SrvSendMsg: false,
		}
	}
	resp, err := b.OpenAPI.PostGroupMessage(context.Background(), groupID, uploadMsg)
	if err != nil {
		b.logError("failed to upload group image: %v", err)
		return err
	}

	// 再发送图文消息（msg_type=7，content=文本，media=图片）
	msg := &dto.MessageToCreate{
		MsgType: dto.RichMediaMsg,
		Content: text,
		Media:   &dto.MediaInfo{FileInfo: resp.FileInfo},
	}
	if _, err := b.OpenAPI.PostGroupMessage(context.Background(), groupID, msg); err != nil {
		b.logError("failed to send group rich media: %v", err)
		return err
	}

	b.Stats.IncSent()
	b.logInfo("sent group rich media to %s: text=%s, image=%s", groupID, text, imageURL)
	return nil
}

func (b *Bot) SendC2CRichMedia(userID, text, imageURL string) error {
	// 先上传图片获取 file_info。data: URL 走 base64 直传（无需公网），http(s) URL 走腾讯回源。
	var uploadMsg *dto.RichMediaMessage
	if b64 := parseDataURL(imageURL); b64 != "" {
		uploadMsg = &dto.RichMediaMessage{
			FileType:   1,
			FileData:   b64,
			SrvSendMsg: false,
		}
	} else {
		uploadMsg = &dto.RichMediaMessage{
			FileType:   1,
			URL:        imageURL,
			SrvSendMsg: false,
		}
	}
	resp, err := b.OpenAPI.PostC2CMessage(context.Background(), userID, uploadMsg)
	if err != nil {
		b.logError("failed to upload C2C image: %v", err)
		return err
	}

	// 再发送图文消息（msg_type=7，content=文本，media=图片）
	msg := &dto.MessageToCreate{
		MsgType: dto.RichMediaMsg,
		Content: text,
		Media:   &dto.MediaInfo{FileInfo: resp.FileInfo},
	}
	if _, err := b.OpenAPI.PostC2CMessage(context.Background(), userID, msg); err != nil {
		b.logError("failed to send C2C rich media: %v", err)
		return err
	}

	b.Stats.IncSent()
	b.logInfo("sent C2C rich media to %s: text=%s, image=%s", userID, text, imageURL)
	return nil
}

// SendGroupMarkdown 发送群 Markdown 消息
func (b *Bot) SendGroupMarkdown(groupID, content string) error {
	msg := &dto.MessageToCreate{
		MsgType:  dto.MarkdownMsg,
		Markdown: &dto.Markdown{Content: content},
	}
	if _, err := b.OpenAPI.PostGroupMessage(context.Background(), groupID, msg); err != nil {
		b.logError("failed to send group markdown: %v", err)
		return err
	}
	b.Stats.IncSent()
	b.logInfo("sent group markdown to %s", groupID)
	return nil
}

// SendC2CMarkdown 发送 C2C Markdown 消息
func (b *Bot) SendC2CMarkdown(userID, content string) error {
	msg := &dto.MessageToCreate{
		MsgType:  dto.MarkdownMsg,
		Markdown: &dto.Markdown{Content: content},
	}
	if _, err := b.OpenAPI.PostC2CMessage(context.Background(), userID, msg); err != nil {
		b.logError("failed to send C2C markdown: %v", err)
		return err
	}
	b.Stats.IncSent()
	b.logInfo("sent C2C markdown to %s", userID)
	return nil
}

// SendGroupKeyboard 发送群 Markdown + 按钮消息
func (b *Bot) SendGroupKeyboard(groupID, markdown, keyboardJSON string) error {
	var kb keyboard.MessageKeyboard
	if err := json.Unmarshal([]byte(keyboardJSON), &kb); err != nil {
		return fmt.Errorf("解析按钮配置失败: %w", err)
	}
	msg := &dto.MessageToCreate{
		MsgType:  dto.MarkdownMsg,
		Markdown: &dto.Markdown{Content: markdown},
		Keyboard: &kb,
	}
	if _, err := b.OpenAPI.PostGroupMessage(context.Background(), groupID, msg); err != nil {
		b.logError("failed to send group keyboard: %v", err)
		return err
	}
	b.Stats.IncSent()
	b.logInfo("sent group keyboard to %s", groupID)
	return nil
}

// SendC2CKeyboard 发送 C2C Markdown + 按钮消息
func (b *Bot) SendC2CKeyboard(userID, markdown, keyboardJSON string) error {
	var kb keyboard.MessageKeyboard
	if err := json.Unmarshal([]byte(keyboardJSON), &kb); err != nil {
		return fmt.Errorf("解析按钮配置失败: %w", err)
	}
	msg := &dto.MessageToCreate{
		MsgType:  dto.MarkdownMsg,
		Markdown: &dto.Markdown{Content: markdown},
		Keyboard: &kb,
	}
	if _, err := b.OpenAPI.PostC2CMessage(context.Background(), userID, msg); err != nil {
		b.logError("failed to send C2C keyboard: %v", err)
		return err
	}
	b.Stats.IncSent()
	b.logInfo("sent C2C keyboard to %s", userID)
	return nil
}

// SendGroupVideo 发送群视频消息
func (b *Bot) SendGroupVideo(groupID, videoURL string) error {
	uploadMsg := &dto.RichMediaMessage{
		FileType:   2, // 2=视频
		URL:        videoURL,
		SrvSendMsg: false,
	}
	resp, err := b.OpenAPI.PostGroupMessage(context.Background(), groupID, uploadMsg)
	if err != nil {
		b.logError("failed to upload group video: %v", err)
		return err
	}
	msg := &dto.MessageToCreate{
		MsgType: dto.RichMediaMsg,
		Media:   &dto.MediaInfo{FileInfo: resp.FileInfo},
	}
	if _, err := b.OpenAPI.PostGroupMessage(context.Background(), groupID, msg); err != nil {
		b.logError("failed to send group video: %v", err)
		return err
	}
	b.Stats.IncSent()
	b.logInfo("sent group video to %s: %s", groupID, videoURL)
	return nil
}

// SendC2CVideo 发送 C2C 视频消息
func (b *Bot) SendC2CVideo(userID, videoURL string) error {
	uploadMsg := &dto.RichMediaMessage{
		FileType:   2, // 2=视频
		URL:        videoURL,
		SrvSendMsg: false,
	}
	resp, err := b.OpenAPI.PostC2CMessage(context.Background(), userID, uploadMsg)
	if err != nil {
		b.logError("failed to upload C2C video: %v", err)
		return err
	}
	msg := &dto.MessageToCreate{
		MsgType: dto.RichMediaMsg,
		Media:   &dto.MediaInfo{FileInfo: resp.FileInfo},
	}
	if _, err := b.OpenAPI.PostC2CMessage(context.Background(), userID, msg); err != nil {
		b.logError("failed to send C2C video: %v", err)
		return err
	}
	b.Stats.IncSent()
	b.logInfo("sent C2C video to %s: %s", userID, videoURL)
	return nil
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
