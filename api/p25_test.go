package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"shizukubot/config"
	"shizukubot/core"
	stats "shizukubot/storage/stats"
	"strings"
	"testing"
	"time"
)

func apiData(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil || body.Code != 0 {
		t.Fatal(w.Body.String(), e)
	}
	return body.Data
}
func imageData(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 7), uint8(y * 11), 170, 255})
		}
	}
	var data bytes.Buffer
	if e := png.Encode(&data, img); e != nil {
		t.Fatal(e)
	}
	return data.Bytes()
}
func TestAppearancePersistencePrivacyRevisionAndUpload(t *testing.T) {
	r, c := setupRouter(t)
	initial := apiData(t, request(r, c, "GET", "/api/config", ""))
	revision := initial["revision"].(string)
	if got := apiData(t, request(r, nil, "GET", "/api/appearance", "")); len(got) != 3 || got["background_url"] != config.DefaultBackground || strings.Contains(mustJSON(got), "password") {
		t.Fatal(got)
	}
	upload := func(data []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/appearance/background", bytes.NewReader(data))
		req.Header.Set("Content-Type", "image/png")
		req.Header.Set("Origin", "http://example.com")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		r.WithCORS().ServeHTTP(w, req)
		return w
	}
	if w := upload(imageData(t), nil); w.Code != 401 {
		t.Fatal("anonymous upload", w.Code, w.Body.String())
	}
	if w := upload([]byte("<svg onload=alert(1)>"), c); !strings.Contains(w.Body.String(), `"code":400`) {
		t.Fatal(w.Body.String())
	}
	if w := upload(make([]byte, (8<<20)+1), c); !strings.Contains(w.Body.String(), `"code":400`) {
		t.Fatal("oversize", w.Body.String())
	}
	// DecodeConfig rejects implausible dimensions before allocating pixels.
	oversized := imageData(t)
	binary.BigEndian.PutUint32(oversized[16:20], 9000)
	if w := upload(oversized, c); strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatal("oversize dimensions accepted")
	}
	uploaded := apiData(t, upload(imageData(t), c))["url"].(string)
	if !config.UploadPattern.MatchString(uploaded) {
		t.Fatal(uploaded)
	}
	if w := request(r, nil, "GET", uploaded, ""); w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatal(w.Code, w.Header())
	}
	for _, url := range []string{"javascript:alert(1)", "data:text/html,x", "http://example.com/a.png", "//evil.test/x.png", "/static/../config/config.json", "/static/%2e%2e/key.png"} {
		body := map[string]interface{}{"revision": revision, "webui": map[string]interface{}{"port": 4545, "appearance": config.Appearance{Background: config.Background{Type: "url", Value: url}, Theme: config.Theme{Mode: "auto"}}}}
		if w := request(r, c, "PUT", "/api/config", mustJSON(body)); !strings.Contains(w.Body.String(), `"code":400`) {
			t.Fatal(url, w.Body.String())
		}
	}
	initial["webui"].(map[string]interface{})["appearance"] = config.Appearance{Background: config.Background{Type: "upload", Value: uploaded}, Theme: config.Theme{Mode: "dark", PrimaryColor: "#0061A4"}}
	body := mustJSON(initial)
	after := apiData(t, request(r, c, "PUT", "/api/config", body))
	if w := request(r, c, "PUT", "/api/config", body); !strings.Contains(w.Body.String(), `"code":409`) {
		t.Fatal("stale config overwritten", w.Body.String())
	}
	if w := request(r, c, "DELETE", "/api/appearance/background", `{"url":"`+uploaded+`"}`); !strings.Contains(w.Body.String(), `"code":409`) {
		t.Fatal("active background deleted")
	}
	saved, err := config.LoadConfig("config/config.json")
	if err != nil || saved.WebUI.Appearance.Theme.PrimaryColor != "#0061A4" || saved.WebUI.Password != "session-signing-secret-for-tests" {
		t.Fatal(saved, err)
	}
	public := apiData(t, request(r, nil, "GET", "/api/appearance", ""))
	if public["mode"] != "dark" || public["background_url"] != uploaded {
		t.Fatal(public)
	}
	if w := request(r, nil, "GET", "/appearance/bootstrap.js", ""); strings.Contains(w.Body.String(), "session-signing") || !strings.Contains(w.Body.String(), "#0061A4") {
		t.Fatal(w.Body.String())
	}
	after["webui"].(map[string]interface{})["appearance"] = config.DefaultAppearance()
	apiData(t, request(r, c, "PUT", "/api/config", mustJSON(after)))
	if _, err = os.Stat(filepath.Join(backgroundDir, filepath.Base(uploaded))); !os.IsNotExist(err) {
		t.Fatal("unused saved background not removed", err)
	}
	// Failed atomic saves keep the active configuration and revision unchanged.
	old := r.configSnapshot()
	r.configPath = "missing-parent-is-file/config.json"
	os.WriteFile("missing-parent-is-file", []byte("x"), 0600)
	current := apiData(t, request(r, c, "GET", "/api/config", ""))
	current["market_api"] = "https://example.invalid/change"
	if w := request(r, c, "PUT", "/api/config", mustJSON(current)); !strings.Contains(w.Body.String(), `"code":500`) {
		t.Fatal(w.Body.String())
	}
	if r.configSnapshot().MarketAPI != old.MarketAPI {
		t.Fatal("failed save published config")
	}
}
func mustJSON(value interface{}) string { data, _ := json.Marshal(value); return string(data) }
func TestPersistentStatisticsAPIAndLiveSettings(t *testing.T) {
	r, c := setupRouter(t)
	store, err := stats.Open("data/stats.db", stats.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	r.botManager.SetStatsStore(store)
	for _, id := range []string{"a", "b"} {
		bot, e := core.NewBot(config.BotConfig{ID: id, AppID: "mock", AppSecret: "mock", Mode: "webhook"})
		if e != nil {
			t.Fatal(e)
		}
		if e = r.botManager.AddBot(bot); e != nil {
			t.Fatal(e)
		}
	}
	a, _ := r.botManager.GetBot("a")
	a.Stats.IncReceived()
	a.Stats.IncSent()
	a.Stats.RecordUser("u")
	a.Stats.RecordGroup("g")
	v := apiData(t, request(r, c, "GET", "/api/stats", ""))
	hours := v["hourly_stats"].([]interface{})
	hour := hours[0].(map[string]interface{})
	if hour["Hour"] != hour["hour"] || hour["Sent"] != hour["sent"] {
		t.Fatal("hourly compatibility lost")
	}
	if v["received_messages"] != float64(1) || v["user_count"] != float64(1) {
		t.Fatal(v)
	}
	if v := apiData(t, request(r, c, "GET", "/api/bots/b/stats", "")); v["user_count"] != float64(0) {
		t.Fatal(v)
	}
	if w := request(r, c, "GET", "/api/stats/history?days=999", ""); !strings.Contains(w.Body.String(), `"code":400`) {
		t.Fatal(w.Body.String())
	}
	old := time.Now().AddDate(0, 0, -10)
	store.Record(stats.Event{Bot: "a", Kind: "sent", At: old})
	cfg := apiData(t, request(r, c, "GET", "/api/config", ""))
	cfg["statistics"] = stats.Options{RetentionDays: 7, CleanupTime: "04:10", Timezone: "Asia/Shanghai"}
	apiData(t, request(r, c, "PUT", "/api/config", mustJSON(cfg)))
	if store.Settings().RetentionDays != 7 || store.Settings().CleanupTime != "04:10" {
		t.Fatal(store.Settings())
	}
	w := request(r, c, "GET", "/api/stats/history?days=90", "")
	var history struct {
		Data []stats.Day `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &history)
	if len(history.Data) != 7 || history.Data[6].Sent != 1 {
		t.Fatal(w.Body.String())
	}
	apiData(t, request(r, c, "POST", "/api/stats/cleanup", "{}"))
	if err = store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.botManager.RemoveBot("a")
	if v := apiData(t, request(r, c, "GET", "/api/stats", "")); v["sent_messages"] != float64(0) {
		t.Fatal("removed bot included", v)
	}
	bot, _ := core.NewBot(config.BotConfig{ID: "a", AppID: "mock", AppSecret: "mock", Mode: "webhook"})
	r.botManager.AddBot(bot)
	if v := apiData(t, request(r, c, "GET", "/api/bots/a/stats", "")); v["user_count"] != float64(1) {
		t.Fatal("recreated identity lost", v)
	}
}
