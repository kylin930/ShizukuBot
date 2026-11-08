package browser

import (
	"context"
	"encoding/base64"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"
)

func requireBrowser(t *testing.T) {
	t.Helper()
	if executable("") == "" {
		t.Skip("no Chrome or Edge found")
	}
}

func TestRenderHTML(t *testing.T) {
	requireBrowser(t)
	html := []byte(`<!doctype html><html><body style="margin:0;background:#123">
<div id="box" style="width:200px;height:100px;color:#fff;font:20px sans-serif">hello</div>
</body></html>`)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	img, err := RenderHTML(ctx, html, RenderConfig{Width: 300, Height: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(img) < 1000 {
		t.Fatalf("screenshot too small: %d bytes", len(img))
	}
	if http.DetectContentType(img) != "image/jpeg" {
		t.Fatalf("unexpected content type %s", http.DetectContentType(img))
	}
	// A second render reuses the browser process after the first tab closed.
	img, err = RenderHTML(ctx, html, RenderConfig{Width: 300, Height: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(img) == 0 {
		t.Fatal("second render returned no image")
	}
	CloseRenderer()
}

func TestRenderHTMLZeroHeight(t *testing.T) {
	requireBrowser(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// Height omitted must fall back to the default 800, not 0: a zero-height
	// window makes Chrome never finish the first navigation.
	img, err := RenderHTML(ctx, []byte(`<!doctype html><html><body>hello</body></html>`), RenderConfig{Width: 800})
	if err != nil {
		t.Fatal(err)
	}
	if len(img) == 0 {
		t.Fatal("empty screenshot")
	}
	CloseRenderer()
}

func TestSessionNavigationAndWatch(t *testing.T) {
	requireBrowser(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s, err := Open(ctx, Config{Width: 400, Height: 300})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	html := base64.StdEncoding.EncodeToString([]byte(`<!doctype html><body>
<div id="qr" style="width:120px;height:120px;background:#000"></div>
</body>`))
	urls := make(chan string, 8)
	stop := s.WatchURLs(func(url, method string) {
		select {
		case urls <- url:
		default:
		}
	})
	defer stop()

	if err := s.Navigate(ctx, "data:text/html;base64,"+html); err != nil {
		t.Fatal(err)
	}
	var width float64
	if err := s.Evaluate(ctx, `document.getElementById('qr').getBoundingClientRect().width`, &width); err != nil {
		t.Fatal(err)
	}
	if width != 120 {
		t.Fatalf("unexpected width %v", width)
	}
	shot, err := s.ElementScreenshot(ctx, "#qr")
	if err != nil {
		t.Fatal(err)
	}
	if len(shot) == 0 {
		t.Fatal("empty element screenshot")
	}
	select {
	case got := <-urls:
		if !strings.HasPrefix(got, "data:text/html") {
			t.Fatalf("unexpected watched URL %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no request observed")
	}
}

func TestFileURL(t *testing.T) {
	if runtime.GOOS == "windows" {
		got := fileURL(`C:\tmp\a b.html`)
		if !strings.HasPrefix(got, "file:///C:/tmp/") {
			t.Fatalf("unexpected file URL %q", got)
		}
		return
	}
	got := fileURL("/tmp/a b.html")
	if got != "file:///tmp/a b.html" {
		t.Fatalf("unexpected file URL %q", got)
	}
}
