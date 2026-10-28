package main

import (
	"crypto/sha256"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// ==================== 数据结构 ====================

type PluginInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Author       string   `json:"author"`
	Category     string   `json:"category"`
	Keywords     []string `json:"keywords"`
	DownloadURL  string   `json:"download_url"`
	Version      string   `json:"version"`
	CreatedAt    int64    `json:"created_at"`
	UpdatedAt    int64    `json:"updated_at"`
	PasswordHash string   `json:"password_hash,omitempty"`
	Downloads    int      `json:"downloads,omitempty"`
}

// 索引文件结构
type IndexFile struct {
	Volumes []VolumeRef `json:"volumes"`
	Total   int         `json:"total"`
	Updated string      `json:"updated"`
}

type VolumeRef struct {
	File  string `json:"file"`
	Count int    `json:"count"`
}

// 卷文件结构
type VolumeFile struct {
	Plugins []PluginInfo `json:"plugins"`
}

const maxPluginsPerVolume = 300
const indexFileKey = "hpuhtrz0vfl291jrv8ay_index.json"

// ==================== STS 凭证 ====================

type STSCreds struct {
	AccessKeyId     string `json:"AccessKeyId"`
	AccessKeySecret string `json:"AccessKeySecret"`
	SecurityToken   string `json:"SecurityToken"`
	Expiration      string `json:"Expiration"`
}

func getSTSCredentials() (*STSCreds, error) {
	token, err := authMgr.GetToken()
	if err != nil {
		return nil, fmt.Errorf("获取认证token失败: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", config.OSS.STSAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求STS API失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("STS API返回错误: %d %s", resp.StatusCode, string(body))
	}

	var creds STSCreds
	if err := json.NewDecoder(resp.Body).Decode(&creds); err != nil {
		return nil, fmt.Errorf("解析STS响应失败: %w", err)
	}

	if creds.AccessKeyId == "" || creds.AccessKeySecret == "" {
		return nil, fmt.Errorf("STS凭证为空")
	}

	return &creds, nil
}

// ==================== OSS 操作 ====================

func ossPutObject(objectKey string, content []byte) error {
	creds, err := getSTSCredentials()
	if err != nil {
		return fmt.Errorf("获取STS凭证失败: %w", err)
	}

	// 优先用公网 endpoint，内网 endpoint 仅在阿里云 ECS 内部部署时配置
	endpoint := config.OSS.Endpoint
	if endpoint == "" {
		endpoint = config.OSS.InternalEndpoint
	}

	client, err := oss.New(endpoint, creds.AccessKeyId, creds.AccessKeySecret, oss.SecurityToken(creds.SecurityToken))
	if err != nil {
		return fmt.Errorf("创建OSS客户端失败: %w", err)
	}

	bucket, err := client.Bucket(config.OSS.BucketName)
	if err != nil {
		return fmt.Errorf("获取Bucket失败: %w", err)
	}

	err = bucket.PutObject(objectKey, strings.NewReader(string(content)), oss.ContentType("application/json"))
	if err != nil {
		return fmt.Errorf("上传对象失败: %w", err)
	}

	return nil
}

func ossGetObject(objectKey string) ([]byte, error) {
	url := config.OSS.PublicBase + "/" + objectKey

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("下载对象失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载对象失败: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取对象内容失败: %w", err)
	}

	return body, nil
}

// ==================== UUID 生成 ====================

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// ==================== 下载计数（内存） ====================

var (
	downloads       = make(map[string]int)
	downloadsMu     sync.RWMutex
	downloadHistory = make(map[string]map[string]downloadRecord)
	downloadHistoryMu sync.Mutex
)

type downloadRecord struct {
	LastDownload time.Time
	CountToday   int
}

// ==================== 插件提交队列（内存） ====================

type PendingPlugin struct {
	PluginInfo
	PasswordHash string `json:"password_hash"`
}

var (
	pendingQueue   []PendingPlugin
	pendingMu      sync.Mutex
	flushInterval  = 15 * time.Minute
	flushRunning   = false
)

func init() {
	go startFlushLoop()
}

func startFlushLoop() {
	if flushRunning {
		return
	}
	flushRunning = true
	for {
		time.Sleep(flushInterval)
		flushPendingPlugins()
	}
}

func addPendingPlugin(p *PluginInfo, password string) {
	pendingMu.Lock()
	defer pendingMu.Unlock()

	p.UpdatedAt = time.Now().Unix()

	for i, pp := range pendingQueue {
		if pp.Author == p.Author && pp.Name == p.Name {
			p.ID = pp.ID
			p.CreatedAt = pp.CreatedAt
			p.Downloads = pp.Downloads
			pendingQueue[i] = PendingPlugin{
				PluginInfo:   *p,
				PasswordHash: hashPassword(password),
			}
			return
		}
	}

	p.ID = fmt.Sprintf("plugin_%d", time.Now().UnixMilli())
	if p.CreatedAt == 0 {
		p.CreatedAt = time.Now().Unix()
	}

	pendingQueue = append(pendingQueue, PendingPlugin{
		PluginInfo:   *p,
		PasswordHash: hashPassword(password),
	})
}

func flushPendingPlugins() {
	pendingMu.Lock()
	if len(pendingQueue) == 0 {
		pendingMu.Unlock()
		return
	}

	plugins := make([]PendingPlugin, len(pendingQueue))
	copy(plugins, pendingQueue)
	pendingQueue = nil
	pendingMu.Unlock()

	for _, pp := range plugins {
		if err := commitPluginToOSS(&pp); err != nil {
			fmt.Printf("❌ Failed to commit plugin %s: %v\n", pp.Name, err)
			pendingMu.Lock()
			pendingQueue = append(pendingQueue, pp)
			pendingMu.Unlock()
		} else {
			fmt.Printf("✅ Committed plugin: %s\n", pp.Name)
		}
	}
}

func commitPluginToOSS(pp *PendingPlugin) error {
	index, err := getIndex()
	if err != nil {
		return fmt.Errorf("获取索引失败: %w", err)
	}

	isUpdate := false
	var volumeFileKey string
	var volume *VolumeFile

	for _, volRef := range index.Volumes {
		v, err := getVolume(volRef.File)
		if err != nil || v == nil {
			continue
		}
		for i, p := range v.Plugins {
			if p.Author == pp.Author && p.Name == pp.Name {
				isUpdate = true
				volumeFileKey = volRef.File
				volume = v
				pp.PluginInfo.Downloads = p.Downloads
				pp.PluginInfo.CreatedAt = p.CreatedAt
				v.Plugins[i] = pp.PluginInfo
				goto updateDone
			}
		}
	}
updateDone:

	if !isUpdate {
		if len(index.Volumes) > 0 {
			last := index.Volumes[len(index.Volumes)-1]
			if last.Count < maxPluginsPerVolume {
				v, err := getVolume(last.File)
				if err != nil {
					return fmt.Errorf("获取卷文件失败: %w", err)
				}
				if v != nil {
					volumeFileKey = last.File
					volume = v
				}
			}
		}

		if volume == nil {
			volumeFileKey = generateUUID() + ".json"
			volume = &VolumeFile{Plugins: []PluginInfo{}}
		}

		volume.Plugins = append(volume.Plugins, pp.PluginInfo)
	}

	volumeContent, _ := json.MarshalIndent(volume, "", "  ")

	if err := ossPutObject(volumeFileKey, volumeContent); err != nil {
		return fmt.Errorf("上传卷文件失败: %w", err)
	}

	found := false
	for i, v := range index.Volumes {
		if v.File == volumeFileKey {
			index.Volumes[i].Count = len(volume.Plugins)
			found = true
			break
		}
	}
	if !found {
		index.Volumes = append(index.Volumes, VolumeRef{
			File:  volumeFileKey,
			Count: len(volume.Plugins),
		})
	}
	if !isUpdate {
		index.Total++
	}
	index.Updated = time.Now().Format(time.RFC3339)

	indexContent, _ := json.MarshalIndent(index, "", "  ")
	if err := ossPutObject(indexFileKey, indexContent); err != nil {
		return fmt.Errorf("上传索引文件失败: %w", err)
	}

	return nil
}

func hashPassword(password string) string {
	h := sha256.New()
	h.Write([]byte(password))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// verifyPluginPassword 校验插件作者密码
// 如果该作者之前提交过插件且设置了密码，则验证密码是否一致
// 如果该作者首次提交或之前未设置密码，则跳过验证
func verifyPluginPassword(author, password string) error {
	index, err := getIndex()
	if err != nil {
		return nil // 获取索引失败时不阻塞提交
	}
	if index.Total == 0 {
		return nil // 市场为空，首次提交
	}

	// 遍历所有卷文件，查找该作者的插件
	for _, volRef := range index.Volumes {
		vol, err := getVolume(volRef.File)
		if err != nil || vol == nil {
			continue
		}
		for _, p := range vol.Plugins {
			if p.Author == author && p.PasswordHash != "" {
				// 该作者之前设过密码，验证
				if p.PasswordHash != hashPassword(password) {
					return fmt.Errorf("密码验证失败：该作者名已存在，请输入正确的密码")
				}
				return nil // 密码正确
			}
		}
	}

	// 也检查待提交队列中的插件
	pendingMu.Lock()
	defer pendingMu.Unlock()
	for _, pp := range pendingQueue {
		if pp.Author == author && pp.PasswordHash != "" {
			if pp.PasswordHash != hashPassword(password) {
				return fmt.Errorf("密码验证失败：该作者名已存在，请输入正确的密码")
			}
			return nil
		}
	}

	return nil // 该作者首次提交或未设密码
}

func increaseDownloads(ip, id string) bool {
	now := time.Now()
	today := now.Truncate(24 * time.Hour)

	downloadHistoryMu.Lock()
	if downloadHistory[ip] == nil {
		downloadHistory[ip] = make(map[string]downloadRecord)
	}

	record := downloadHistory[ip][id]
	
	if record.CountToday > 0 && record.LastDownload.Add(30*time.Second).After(now) {
		downloadHistoryMu.Unlock()
		return false
	}

	if record.LastDownload.Before(today) {
		record.CountToday = 0
	}

	if record.CountToday >= 50 {
		downloadHistoryMu.Unlock()
		return false
	}

	record.LastDownload = now
	record.CountToday++
	downloadHistory[ip][id] = record
	downloadHistoryMu.Unlock()

	downloadsMu.Lock()
	downloads[id]++
	downloadsMu.Unlock()

	go func() {
		err := updatePluginDownloads(id)
		if err != nil {
			fmt.Printf("❌ Failed to update downloads for plugin %s: %v\n", id, err)
		}
	}()

	return true
}

func updatePluginDownloads(id string) error {
	index, err := getIndex()
	if err != nil {
		return fmt.Errorf("获取索引失败: %w", err)
	}

	for _, volRef := range index.Volumes {
		v, err := getVolume(volRef.File)
		if err != nil || v == nil {
			continue
		}
		for i, p := range v.Plugins {
			if p.ID == id {
				v.Plugins[i].Downloads++
				v.Plugins[i].UpdatedAt = time.Now().Unix()

				volumeContent, _ := json.MarshalIndent(v, "", "  ")
				if err := ossPutObject(volRef.File, volumeContent); err != nil {
					return fmt.Errorf("更新卷文件失败: %w", err)
				}
				return nil
			}
		}
	}

	return fmt.Errorf("插件 %s 未找到", id)
}

func getDownloadCount(id string) int {
	downloadsMu.RLock()
	defer downloadsMu.RUnlock()
	return downloads[id]
}

// ==================== 插件存储逻辑 ====================

func addPlugin(p *PluginInfo) error {
	p.ID = fmt.Sprintf("plugin_%d", time.Now().UnixMilli())
	if p.CreatedAt == 0 {
		p.CreatedAt = time.Now().Unix()
	}
	p.UpdatedAt = time.Now().Unix()

	index, err := getIndex()
	if err != nil {
		return fmt.Errorf("获取索引失败: %w", err)
	}

	var volumeFileKey string
	var volume *VolumeFile

	if len(index.Volumes) > 0 {
		last := index.Volumes[len(index.Volumes)-1]
		if last.Count < maxPluginsPerVolume {
			v, err := getVolume(last.File)
			if err != nil {
				return fmt.Errorf("获取卷文件失败: %w", err)
			}
			if v != nil {
				volumeFileKey = last.File
				volume = v
			}
		}
	}

	if volume == nil {
		volumeFileKey = generateUUID() + ".json"
		volume = &VolumeFile{Plugins: []PluginInfo{}}
	}

	volume.Plugins = append(volume.Plugins, *p)
	volumeContent, _ := json.MarshalIndent(volume, "", "  ")

	if err := ossPutObject(volumeFileKey, volumeContent); err != nil {
		return fmt.Errorf("上传卷文件失败: %w", err)
	}

	found := false
	for i, v := range index.Volumes {
		if v.File == volumeFileKey {
			index.Volumes[i].Count = len(volume.Plugins)
			found = true
			break
		}
	}
	if !found {
		index.Volumes = append(index.Volumes, VolumeRef{
			File:  volumeFileKey,
			Count: len(volume.Plugins),
		})
	}
	index.Total++
	index.Updated = time.Now().Format(time.RFC3339)

	indexContent, _ := json.MarshalIndent(index, "", "  ")
	if err := ossPutObject(indexFileKey, indexContent); err != nil {
		return fmt.Errorf("上传索引文件失败: %w", err)
	}

	return nil
}

// getIndex 从 OSS 获取索引文件
func getIndex() (*IndexFile, error) {
	data, err := ossGetObject(indexFileKey)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return &IndexFile{
			Volumes: []VolumeRef{},
			Total:   0,
			Updated: time.Now().Format(time.RFC3339),
		}, nil
	}

	var index IndexFile
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if index.Volumes == nil {
		index.Volumes = []VolumeRef{}
	}
	return &index, nil
}

// getVolume 从 OSS 获取卷文件
func getVolume(objectKey string) (*VolumeFile, error) {
	data, err := ossGetObject(objectKey)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return &VolumeFile{Plugins: []PluginInfo{}}, nil
	}

	var vol VolumeFile
	if err := json.Unmarshal(data, &vol); err != nil {
		return nil, err
	}
	if vol.Plugins == nil {
		vol.Plugins = []PluginInfo{}
	}
	return &vol, nil
}

// ==================== 仓库初始化 ====================

func InitStorage() error {
	if config.OSS.PublicBase == "" || config.OSS.BucketName == "" {
		fmt.Println("⚠️ OSS config not complete, storage will not work")
		return nil
	}

	index, err := getIndex()
	if err != nil {
		return fmt.Errorf("failed to access OSS: %w", err)
	}
	if index.Total == 0 {
		fmt.Println("📦 Plugin market is empty, will create index on first upload")
	} else {
		fmt.Printf("📦 Plugin market loaded: %d plugins in %d volumes\n", index.Total, len(index.Volumes))
	}

	return nil
}

// ==================== 统计 ====================

func getStats() map[string]interface{} {
	index, err := getIndex()
	totalPlugins := 0
	if err == nil {
		totalPlugins = index.Total
	}

	downloadsMu.RLock()
	totalDownloads := 0
	for _, n := range downloads {
		totalDownloads += n
	}
	downloadsMu.RUnlock()

	return map[string]interface{}{
		"total_plugins":   totalPlugins,
		"total_downloads": totalDownloads,
	}
}

// ==================== 搜索建议（服务端可选） ====================

// getSearchSuggestions 服务端简单搜索（基于卷文件）
// 主要搜索由客户端完成，这里仅作备用
func getSearchSuggestions(query string) ([]PluginInfo, error) {
	index, err := getIndex()
	if err != nil {
		return nil, err
	}

	var results []PluginInfo
	for _, vol := range index.Volumes {
		v, err := getVolume(vol.File)
		if err != nil {
			continue
		}
		for _, p := range v.Plugins {
			if strings.Contains(p.Name, query) ||
				strings.Contains(p.Description, query) ||
				strings.Contains(p.Author, query) {
				results = append(results, p)
				if len(results) >= 20 {
					return results, nil
				}
			}
		}
	}
	return results, nil
}
