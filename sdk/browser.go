package sdk

import (
	"context"
	"errors"
	"time"

	"shizukubot/browser"
)

// RenderOptions controls one HTML-to-image render.
type RenderOptions struct {
	// Width and Height set the viewport; zero means 1280x800.
	Width  int
	Height int
	// FullPage captures the whole document instead of the viewport.
	FullPage bool
	// Timeout bounds navigation and capture; zero means 60 seconds.
	Timeout time.Duration
}

// RenderHTML screenshots HTML with the host's local Chrome or Edge. Cards and
// reports use this instead of shipping an image renderer inside the plugin.
func RenderHTML(ctx context.Context, html []byte, opts RenderOptions) ([]byte, error) {
	return browser.RenderHTML(ctx, html, browser.RenderConfig{
		Width:    opts.Width,
		Height:   opts.Height,
		FullPage: opts.FullPage,
		Timeout:  opts.Timeout,
	})
}

// BrowserOptions configures a scriptable login or scrape session.
type BrowserOptions struct {
	Width            int
	Height           int
	Mobile           bool
	ShowWindow       bool
	UserAgent        string
	ExecPath         string
	Proxy            string
	IgnoreCertErrors bool
	Timeout          time.Duration
}

// Browser is a single tab whose page can be driven from a plugin. The
// underlying browser process is released by Close.
type Browser struct {
	session *browser.Session
}

// OpenBrowser starts a browser tab. Login flows that need the same session for
// QR display, polling and redirect interception use this.
func OpenBrowser(ctx context.Context, opts BrowserOptions) (*Browser, error) {
	session, err := browser.Open(ctx, browser.Config{
		Width:            opts.Width,
		Height:           opts.Height,
		Mobile:           opts.Mobile,
		ShowWindow:       opts.ShowWindow,
		UserAgent:        opts.UserAgent,
		ExecPath:         opts.ExecPath,
		Proxy:            opts.Proxy,
		IgnoreCertErrors: opts.IgnoreCertErrors,
		Timeout:          opts.Timeout,
	})
	if err != nil {
		return nil, err
	}
	return &Browser{session: session}, nil
}

func (b *Browser) owner() (*browser.Session, error) {
	if b == nil || b.session == nil {
		return nil, errors.New("browser is closed")
	}
	return b.session, nil
}

// Navigate loads url and waits for the load event.
func (b *Browser) Navigate(ctx context.Context, url string) error {
	s, err := b.owner()
	if err != nil {
		return err
	}
	return s.Navigate(ctx, url)
}

// Evaluate runs JavaScript and unmarshals its return value into out.
func (b *Browser) Evaluate(ctx context.Context, expr string, out any) error {
	s, err := b.owner()
	if err != nil {
		return err
	}
	return s.Evaluate(ctx, expr, out)
}

// EvaluateAwait runs JavaScript that returns a Promise.
func (b *Browser) EvaluateAwait(ctx context.Context, expr string, out any) error {
	s, err := b.owner()
	if err != nil {
		return err
	}
	return s.EvaluateAwait(ctx, expr, out)
}

// WaitVisible waits until the CSS selector matches a visible element.
func (b *Browser) WaitVisible(ctx context.Context, selector string) error {
	s, err := b.owner()
	if err != nil {
		return err
	}
	return s.WaitVisible(ctx, selector)
}

// HTML returns the current document markup.
func (b *Browser) HTML(ctx context.Context) (string, error) {
	s, err := b.owner()
	if err != nil {
		return "", err
	}
	return s.HTML(ctx)
}

// Screenshot captures the viewport as JPEG.
func (b *Browser) Screenshot(ctx context.Context) ([]byte, error) {
	s, err := b.owner()
	if err != nil {
		return nil, err
	}
	return s.Screenshot(ctx)
}

// ElementScreenshot captures one CSS-selected element as PNG.
func (b *Browser) ElementScreenshot(ctx context.Context, selector string) ([]byte, error) {
	s, err := b.owner()
	if err != nil {
		return nil, err
	}
	return s.ElementScreenshot(ctx, selector)
}

// WatchURLs reports every request the page makes, including custom-scheme
// redirects such as auth://tauth.qq.com/?code=... The handler must return
// quickly; call the returned function to stop watching.
func (b *Browser) WatchURLs(handler func(url, method string)) func() {
	s, err := b.owner()
	if err != nil {
		return func() {}
	}
	return s.WatchURLs(handler)
}

// Close releases the tab and its browser process.
func (b *Browser) Close() {
	if b != nil && b.session != nil {
		b.session.Close()
		b.session = nil
	}
}

// CloseRenderBrowser shuts down the shared renderer and its browser process.
func CloseRenderBrowser() { browser.CloseRenderer() }
