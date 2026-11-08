// Package browser runs a local Chrome or Edge instance through the Chrome
// DevTools Protocol. It backs sdk.RenderHTML and sdk.OpenBrowser; plugins never
// import chromedp directly.
package browser

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// DefaultTimeout bounds one navigation or one batch of actions.
const DefaultTimeout = 60 * time.Second

// Config describes one browser process. The zero value works: it uses the
// detected Chrome/Edge in headless mode at 1280x800.
type Config struct {
	Width             int
	Height            int
	DeviceScaleFactor float64
	Mobile            bool
	ShowWindow        bool
	UserAgent         string
	ExecPath          string
	Proxy             string
	IgnoreCertErrors  bool
	Timeout           time.Duration
}

// Session is one browser tab with its own DevTools connection.
type Session struct {
	ctx     context.Context
	cancel  context.CancelFunc
	timeout time.Duration
	once    sync.Once
}

func normalize(cfg Config) Config {
	if cfg.Width <= 0 {
		cfg.Width = 1280
	}
	if cfg.Height <= 0 {
		cfg.Height = 800
	}
	if cfg.DeviceScaleFactor <= 0 {
		cfg.DeviceScaleFactor = 1
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return cfg
}

// Open starts a browser process and returns a ready-to-use tab.
func Open(ctx context.Context, cfg Config) (*Session, error) {
	cfg = normalize(cfg)
	opts := allocatorOptions(cfg)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	s := &Session{ctx: tabCtx, timeout: cfg.Timeout}
	s.cancel = func() {
		tabCancel()
		allocCancel()
	}

	var actions []chromedp.Action
	if cfg.Mobile {
		actions = append(actions, chromedp.EmulateViewport(int64(cfg.Width), int64(cfg.Height), chromedp.EmulateMobile))
	} else if cfg.DeviceScaleFactor != 1 {
		actions = append(actions, chromedp.EmulateViewport(int64(cfg.Width), int64(cfg.Height),
			chromedp.EmulateScale(cfg.DeviceScaleFactor)))
	}
	actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.Enable().Do(ctx)
	}))
	// The first Run allocates the browser and ties its lifetime to this
	// context, so it must not run under a timeout child.
	if err := chromedp.Run(tabCtx, actions...); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func allocatorOptions(cfg Config) []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.WindowSize(cfg.Width, cfg.Height),
	}
	if !cfg.ShowWindow {
		opts = append(opts, chromedp.Headless)
	}
	if cfg.UserAgent != "" {
		opts = append(opts, chromedp.UserAgent(cfg.UserAgent))
	}
	if cfg.Proxy != "" {
		opts = append(opts, chromedp.ProxyServer(cfg.Proxy))
	}
	if cfg.IgnoreCertErrors {
		opts = append(opts, chromedp.Flag("ignore-certificate-errors", true))
	}
	if exe := executable(cfg.ExecPath); exe != "" {
		opts = append(opts, chromedp.ExecPath(exe))
	}
	return opts
}

// executable prefers an explicit path, then SHIZUKUBOT_BROWSER_PATH, then the
// usual Chrome/Edge install locations, then PATH.
func executable(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv("SHIZUKUBOT_BROWSER_PATH"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		for _, dir := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
			if dir == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(dir, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(dir, "Microsoft", "Edge", "Application", "msedge.exe"))
		}
		if local != "" {
			candidates = append(candidates,
				filepath.Join(local, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(local, "Microsoft", "Edge", "Application", "msedge.exe"))
		}
	case "darwin":
		candidates = append(candidates,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge")
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	for _, name := range []string{"chrome", "google-chrome", "chromium", "chromium-browser", "msedge", "microsoft-edge"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// run applies actions with the session timeout. The timeout context derives
// from the tab context so the DevTools target stays attached.
func (s *Session) run(ctx context.Context, actions ...chromedp.Action) error {
	if s.ctx == nil {
		return errors.New("browser session is closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(s.ctx, s.timeout)
	defer cancel()
	return chromedp.Run(runCtx, actions...)
}

// Navigate loads url and waits for the load event.
func (s *Session) Navigate(ctx context.Context, url string) error {
	return s.run(ctx, chromedp.Navigate(url))
}

// Evaluate runs JavaScript and unmarshals its return value into out.
func (s *Session) Evaluate(ctx context.Context, expr string, out any) error {
	return s.run(ctx, chromedp.Evaluate(expr, out))
}

// EvaluateAwait is Evaluate for expressions that return a Promise.
func (s *Session) EvaluateAwait(ctx context.Context, expr string, out any) error {
	var raw json.RawMessage
	err := s.run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		v, exception, err := cdpruntime.Evaluate(expr).
			WithAwaitPromise(true).
			WithReturnByValue(true).
			Do(ctx)
		if err != nil {
			return err
		}
		if exception != nil {
			return exception
		}
		if v == nil || len(v.Value) == 0 {
			return nil
		}
		raw = append(raw[:0], v.Value...)
		return nil
	}))
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// WaitVisible waits until the CSS selector matches a visible element.
func (s *Session) WaitVisible(ctx context.Context, selector string) error {
	return s.run(ctx, chromedp.WaitVisible(selector, chromedp.ByQuery))
}

// HTML returns the current document markup.
func (s *Session) HTML(ctx context.Context) (string, error) {
	var markup string
	if err := s.run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.OuterHTML("html", &markup, chromedp.ByQuery).Do(ctx)
	})); err != nil {
		return "", err
	}
	return markup, nil
}

// Screenshot captures the visible viewport as JPEG.
func (s *Session) Screenshot(ctx context.Context) ([]byte, error) {
	var buf []byte
	err := s.run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		data, err := page.CaptureScreenshot().
			WithFormat(page.CaptureScreenshotFormatJpeg).
			WithQuality(90).
			WithFromSurface(true).
			Do(ctx)
		buf = data
		return err
	}))
	return buf, err
}

// ElementScreenshot captures one element as PNG.
func (s *Session) ElementScreenshot(ctx context.Context, selector string) ([]byte, error) {
	var buf []byte
	if err := s.run(ctx, chromedp.Screenshot(selector, &buf, chromedp.ByQuery, chromedp.NodeVisible)); err != nil {
		return nil, err
	}
	return buf, nil
}

// WatchURLs reports every request the page makes. fn runs on the event loop
// goroutine and must return quickly; forward to a channel if work is slow.
// Call the returned function to stop watching.
func (s *Session) WatchURLs(fn func(url, method string)) func() {
	done := make(chan struct{})
	var once sync.Once
	chromedp.ListenTarget(s.ctx, func(ev any) {
		select {
		case <-done:
			return
		default:
		}
		e, ok := ev.(*network.EventRequestWillBeSent)
		if !ok || e.Request == nil {
			return
		}
		fn(e.Request.URL, string(e.Request.Method))
	})
	return func() { once.Do(func() { close(done) }) }
}

// Close releases the tab and, for a session from Open, the browser process.
func (s *Session) Close() {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
		s.ctx = nil
	})
}

// RenderConfig describes one HTML-to-image render.
type RenderConfig struct {
	Width    int
	Height   int
	FullPage bool
	Timeout  time.Duration
}

var renderer struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

// RenderHTML screenshots html bytes with a shared headless browser. Each call
// gets its own tab; the browser process is started once and reused.
func RenderHTML(ctx context.Context, html []byte, cfg RenderConfig) ([]byte, error) {
	if len(html) == 0 {
		return nil, errors.New("empty HTML")
	}
	normalized := normalize(Config{Width: cfg.Width, Height: cfg.Height, Timeout: cfg.Timeout})
	cfg.Width, cfg.Height, cfg.Timeout = normalized.Width, normalized.Height, normalized.Timeout

	s, release, err := renderSession(cfg)
	if err != nil {
		return nil, err
	}
	defer release()

	file, err := os.CreateTemp("", "shizukubot-render-*.html")
	if err != nil {
		return nil, err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.Write(html); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}

	var buf []byte
	actions := []chromedp.Action{
		chromedp.Navigate(fileURL(path)),
		chromedp.WaitReady("body", chromedp.ByQuery),
		waitDocuments(),
	}
	if cfg.FullPage {
		actions = append(actions, chromedp.FullScreenshot(&buf, 90))
	} else {
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			data, err := page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatJpeg).
				WithQuality(90).
				WithFromSurface(true).
				Do(ctx)
			buf = data
			return err
		}))
	}
	if err := s.run(ctx, actions...); err != nil {
		return nil, err
	}
	return buf, nil
}

// renderSession returns a new tab on the shared browser. release closes only
// the tab; the browser stays alive for the next render.
func renderSession(cfg RenderConfig) (*Session, func(), error) {
	renderer.mu.Lock()
	defer renderer.mu.Unlock()
	if renderer.ctx == nil {
		allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocatorOptions(Config{
			Width:  cfg.Width,
			Height: cfg.Height,
		})...)
		ownerCtx, ownerCancel := chromedp.NewContext(allocCtx)
		// Allocate the browser on the owner context so it survives every tab.
		enable := chromedp.ActionFunc(func(ctx context.Context) error {
			return network.Enable().Do(ctx)
		})
		if err := chromedp.Run(ownerCtx, enable); err != nil {
			ownerCancel()
			allocCancel()
			return nil, nil, err
		}
		renderer.ctx = ownerCtx
		renderer.cancel = func() {
			ownerCancel()
			allocCancel()
		}
	}
	tabCtx, tabCancel := chromedp.NewContext(renderer.ctx)
	return &Session{ctx: tabCtx, timeout: cfg.Timeout}, tabCancel, nil
}

// CloseRenderer shuts down the shared renderer and its browser process.
func CloseRenderer() {
	renderer.mu.Lock()
	defer renderer.mu.Unlock()
	if renderer.cancel != nil {
		renderer.cancel()
		renderer.cancel = nil
		renderer.ctx = nil
	}
}

// waitDocuments waits briefly for images and web fonts so cards are not
// captured half-drawn. It gives up instead of failing the whole render.
func waitDocuments() chromedp.Action {
	const js = `(() => {
		if (document.readyState !== 'complete') return false;
		for (const img of document.images) {
			if (!img.complete || img.naturalWidth === 0) return false;
		}
		return document.fonts ? document.fonts.status === 'loaded' : true;
	})()`
	return chromedp.ActionFunc(func(ctx context.Context) error {
		deadline := time.Now().Add(10 * time.Second)
		for {
			var ready bool
			if err := chromedp.Evaluate(js, &ready).Do(ctx); err == nil && ready {
				return nil
			}
			if time.Now().After(deadline) {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	})
}

func fileURL(path string) string {
	p := filepath.ToSlash(path)
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			p = filepath.ToSlash(abs)
		}
	}
	if len(p) > 0 && p[0] != '/' {
		p = "/" + p
	}
	return "file://" + p
}
