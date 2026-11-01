package webui

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMutationOriginPolicy(t *testing.T) {
	ConfigureCookies(false)
	next := SecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	cases := []struct {
		origin, referer, site, typ string
		want                       int
	}{
		{"http://example.com", "", "", "application/json", 204}, {"https://evil.invalid", "", "", "application/json", 403}, {"", "http://evil.invalid/x", "", "application/json", 403}, {"", "", "cross-site", "application/json", 403}, {"", "", "", "text/plain", 415}, {"", "", "", "application/json", 204},
	}
	for _, tt := range cases {
		req := httptest.NewRequest("POST", "http://example.com/api/config", strings.NewReader(`{}`))
		req.Header.Set("Origin", tt.origin)
		req.Header.Set("Referer", tt.referer)
		req.Header.Set("Sec-Fetch-Site", tt.site)
		req.Header.Set("Content-Type", tt.typ)
		w := httptest.NewRecorder()
		next.ServeHTTP(w, req)
		if w.Code != tt.want {
			t.Fatalf("%+v got %d", tt, w.Code)
		}
	}
}
func TestCookieTLSConfigurationAndUntrustedHeaders(t *testing.T) {
	ConfigureCookies(false)
	defer ConfigureCookies(false)
	InitSessionStore("test-session-signing-secret")
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Origin", "https://example.com")
	if SameOrigin(req) {
		t.Fatal("trusted forged forwarded protocol")
	}
	req.TLS = &tls.ConnectionState{}
	if !SameOrigin(req) {
		t.Fatal("actual TLS not recognized")
	}
	ConfigureCookies(true)
	req.TLS = nil
	if !SameOrigin(req) {
		t.Fatal("explicit TLS termination policy not honored")
	}
	InitSessionStore("test-session-signing-secret")
	if !store.Options.Secure || store.Options.SameSite != http.SameSiteLaxMode || !store.Options.HttpOnly {
		t.Fatal("production cookie policy")
	}
}
