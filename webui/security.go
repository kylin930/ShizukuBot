package webui

import (
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// SameOrigin validates browser origins against the actual host, never proxy headers.
func SameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Host == "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil || forceSecureCookies {
		scheme = "https"
	}
	return u.Scheme == scheme && strings.EqualFold(u.Host, r.Host)
}

// SecurityMiddleware preserves same-origin browser forms and native JSON clients.
// QQ webhooks use independent signatures and do not use browser session authentication.
func SecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/login" {
			w.Header().Set("Cache-Control", "no-store")
		}
		if strings.HasPrefix(r.URL.Path, "/webhook/") {
			next.ServeHTTP(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		unsafe := r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS"
		if unsafe {
			if !SameOrigin(r) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "cross-origin request rejected", 403)
				return
			}
			typ, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			// Origin-less requests must not use browser simple-request encodings.
			if r.Header.Get("Origin") == "" && r.Header.Get("Referer") == "" && typ != "application/json" {
				http.Error(w, "JSON required", 415)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// AdminMiddleware protects application administration without introducing full RBAC.
func AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := GetCurrentUser(r)
		if !ok {
			http.Error(w, "authentication required", 401)
			return
		}
		if !u.IsAdmin && r.URL.Path != "/api/user/me" {
			http.Error(w, "administrator required", 403)
			return
		}
		next.ServeHTTP(w, r)
	})
}
