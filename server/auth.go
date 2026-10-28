package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"sync"
	"time"
)

type AuthManager struct {
	token     string
	expiresAt int64
	mu        sync.RWMutex
	cacheFile string
}

var authMgr *AuthManager

func InitAuth() error {
	authMgr = &AuthManager{
		cacheFile: config.Auth.CacheFile,
	}
	return authMgr.loadCache()
}

func (m *AuthManager) loadCache() error {
	if config.Auth.Phone == "" || config.Auth.Password == "" {
		fmt.Println("⚠️ Auth credentials not configured, STS will not work")
		return nil
	}

	if _, err := os.Stat(m.cacheFile); os.IsNotExist(err) {
		return m.refreshToken()
	}

	data, err := os.ReadFile(m.cacheFile)
	if err != nil {
		return m.refreshToken()
	}

	var cache struct {
		Token     string `json:"token"`
		CreatedAt int64  `json:"created_at"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		return m.refreshToken()
	}

	safeBuffer := int64(24 * 3600)
	if time.Now().Unix() < cache.ExpiresAt-safeBuffer {
		m.mu.Lock()
		m.token = cache.Token
		m.expiresAt = cache.ExpiresAt
		m.mu.Unlock()
		fmt.Println("✅ Loaded auth token from cache")
		return nil
	}

	return m.refreshToken()
}

func (m *AuthManager) GetToken() (string, error) {
	m.mu.RLock()
	safeBuffer := int64(24 * 3600)
	if m.token != "" && time.Now().Unix() < m.expiresAt-safeBuffer {
		t := m.token
		m.mu.RUnlock()
		return t, nil
	}
	m.mu.RUnlock()

	if err := m.refreshToken(); err != nil {
		return "", err
	}

	m.mu.RLock()
	t := m.token
	m.mu.RUnlock()
	return t, nil
}

func (m *AuthManager) refreshToken() error {
	if config.Auth.Phone == "" || config.Auth.Password == "" {
		return fmt.Errorf("auth credentials not configured")
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("phone", config.Auth.Phone)
	writer.WriteField("password", config.Auth.Password)
	writer.Close()

	req, err := http.NewRequest("POST", config.Auth.LoginURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36 Edge/148.0.0.0")
	req.Header.Set("Origin", "https://zxzj.ciallo.uk")
	req.Header.Set("Referer", "https://zxzj.ciallo.uk/")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Errcode int    `json:"errcode"`
		Errstr  string `json:"errstr"`
		Data    struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse login response failed: %w", err)
	}

	if result.Errcode != 0 {
		return fmt.Errorf("login failed: %s", result.Errstr)
	}

	token := result.Data.Token
	if token == "" {
		return fmt.Errorf("login response has no token")
	}

	expiresAt := time.Now().Unix() + 14*24*3600

	m.mu.Lock()
	m.token = token
	m.expiresAt = expiresAt
	m.mu.Unlock()

	cacheData := map[string]interface{}{
		"token":      token,
		"created_at": time.Now().Unix(),
		"expires_at": expiresAt,
	}
	cacheJSON, _ := json.MarshalIndent(cacheData, "", "  ")
	os.WriteFile(m.cacheFile, cacheJSON, 0644)

	fmt.Println("✅ Auth token refreshed successfully")
	return nil
}
