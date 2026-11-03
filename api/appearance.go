package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gorilla/mux"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"shizukubot/config"
	"shizukubot/core"
	"time"
)

const backgroundDir = "data/appearance"
const maxBackgroundBytes = 8 << 20

func (r *Router) appearancePublic() map[string]string {
	a := r.configSnapshot().WebUI.Appearance
	return map[string]string{"background_url": a.BackgroundURL(), "primary_color": a.Theme.PrimaryColor, "mode": a.Theme.Mode}
}
func (r *Router) PublicAppearance(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	SuccessResponse(w, r.appearancePublic())
}
func (r *Router) AppearanceBootstrap(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	data, _ := json.Marshal(r.appearancePublic())
	w.Write([]byte("window.ShizukuAppearance="))
	w.Write(data)
	w.Write([]byte(";"))
}
func (r *Router) validateBackgroundFile(b config.Background) error {
	if b.Type != "upload" {
		return nil
	}
	info, err := os.Lstat(filepath.Join(backgroundDir, filepath.Base(b.Value)))
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("上传的背景不存在，请重新上传")
	}
	return nil
}
func (r *Router) removeBackground(value string) error {
	if !config.UploadPattern.MatchString(value) {
		return errors.New("invalid uploaded background")
	}
	err := os.Remove(filepath.Join(backgroundDir, filepath.Base(value)))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (r *Router) UploadBackground(w http.ResponseWriter, req *http.Request) {
	req.Body = http.MaxBytesReader(w, req.Body, maxBackgroundBytes)
	data, err := io.ReadAll(req.Body)
	if err != nil {
		BadRequestResponse(w, "图片不能超过 8 MiB")
		return
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	ext := map[string]string{"png": "png", "jpeg": "jpg", "webp": "webp"}[format]
	if err != nil || ext == "" {
		BadRequestResponse(w, "只支持 PNG、JPEG 和 WebP 图片")
		return
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
		BadRequestResponse(w, "图片尺寸不能超过 4096×4096 或 1600 万像素")
		return
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		BadRequestResponse(w, "图片内容无效")
		return
	}
	r.appearanceMu.Lock()
	defer r.appearanceMu.Unlock()
	if err = os.MkdirAll(backgroundDir, 0700); err != nil {
		InternalErrorResponse(w, "图片保存失败")
		return
	}
	active := r.configSnapshot().WebUI.Appearance.Background.Value
	entries, err := os.ReadDir(backgroundDir)
	if err != nil {
		InternalErrorResponse(w, "图片目录不可读")
		return
	}
	count := 0
	for _, entry := range entries {
		url := "/appearance/images/" + entry.Name()
		if !config.UploadPattern.MatchString(url) {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			continue
		}
		if url != active && time.Since(info.ModTime()) > 24*time.Hour {
			if e = os.Remove(filepath.Join(backgroundDir, entry.Name())); e != nil {
				r.logManager.Error("", "unused background cleanup failed")
			}
		} else {
			count++
		}
	}
	if count >= 32 {
		BadRequestResponse(w, "待保存图片过多，请删除未使用图片后重试")
		return
	}
	var key [16]byte
	if _, err = rand.Read(key[:]); err != nil {
		InternalErrorResponse(w, "图片保存失败")
		return
	}
	name := hex.EncodeToString(key[:]) + "." + ext
	file, err := os.OpenFile(filepath.Join(backgroundDir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		InternalErrorResponse(w, "图片保存失败")
		return
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		os.Remove(filepath.Join(backgroundDir, name))
		InternalErrorResponse(w, "图片保存失败")
		return
	}
	SuccessResponse(w, map[string]interface{}{"url": "/appearance/images/" + name, "width": cfg.Width, "height": cfg.Height})
}
func (r *Router) DeleteBackground(w http.ResponseWriter, req *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil || !config.UploadPattern.MatchString(body.URL) {
		BadRequestResponse(w, "invalid background")
		return
	}
	r.appearanceMu.Lock()
	defer r.appearanceMu.Unlock()
	if r.configSnapshot().WebUI.Appearance.Background.Value == body.URL {
		ErrorResponse(w, 409, "正在使用的背景不能删除，请先恢复默认")
		return
	}
	if err := r.removeBackground(body.URL); err != nil {
		InternalErrorResponse(w, "图片删除失败")
		return
	}
	SuccessResponse(w, nil)
}
func (r *Router) BackgroundImage(w http.ResponseWriter, req *http.Request) {
	name := mux.Vars(req)["file"]
	if !config.UploadPattern.MatchString("/appearance/images/" + name) {
		http.NotFound(w, req)
		return
	}
	path := filepath.Join(backgroundDir, name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, req)
		return
	}
	types := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".webp": "image/webp"}
	w.Header().Set("Content-Type", types[filepath.Ext(name)])
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, req, path)
}
func (r *Router) SystemInfo(w http.ResponseWriter, req *http.Request) {
	sdk := "unknown"
	if build, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range build.Deps {
			if dep.Path == "github.com/kylin930/botgo-plus" {
				sdk = dep.Version
			}
		}
	}
	SuccessResponse(w, map[string]string{"core_version": core.CoreVersion, "webui_version": core.WebUIVersion, "go_version": core.GoVersion(), "platform": core.CoreOS(), "uptime": core.CoreUptime(), "sdk": "botgo-plus", "sdk_version": sdk})
}
