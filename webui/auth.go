package webui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/sessions"
)

const (
	sessionName    = "shizukubot_session"
	sessionUserKey = "username"
)

var storeMu sync.RWMutex
var store *sessions.CookieStore

func sessionStore() *sessions.CookieStore { storeMu.RLock(); defer storeMu.RUnlock(); return store }

var forceSecureCookies bool

// ConfigureCookies explicitly enables secure cookies behind a trusted HTTPS terminator.
func ConfigureCookies(secure bool) { forceSecureCookies = secure }

func InitSessionStore(secret string) {
	storeMu.Lock()
	defer storeMu.Unlock()
	store = sessions.NewCookieStore([]byte(secret))
	store.Options = &sessions.Options{
		HttpOnly: true,
		Secure:   forceSecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
		Path:     "/",
	}
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	var username, password string

	contentType := r.Header.Get("Content-Type")
	if len(contentType) >= 16 && contentType[:16] == "application/json" {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    1,
				"message": "请求格式错误",
			})
			return
		}
		username = body.Username
		password = body.Password
	} else {
		r.ParseForm()
		username = r.FormValue("username")
		password = r.FormValue("password")
	}

	user, ok := userStore.Authenticate(username, password)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "用户名或密码错误",
		})
		return
	}

	session, err := sessionStore().Get(r, sessionName)
	if err != nil {
		session, err = sessionStore().New(r, sessionName)
		if err != nil {
			http.Error(w, "session failure", 500)
			return
		}
	}

	session.Options.Secure = forceSecureCookies || r.TLS != nil
	session.Values[sessionUserKey] = user.Username
	session.Values["auth_tag"] = authTag(user)
	if err := session.Save(r, w); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "服务器内部错误",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "登录成功",
		"data": map[string]interface{}{
			"username": user.Username,
			"is_admin": user.IsAdmin,
		},
	})
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	session, err := sessionStore().Get(r, sessionName)
	if err != nil {
		http.Error(w, "session failure", http.StatusInternalServerError)
		return
	}

	session.Values[sessionUserKey] = nil
	session.Options.MaxAge = -1
	if err := session.Save(r, w); err != nil {
		http.Error(w, "session failure", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/login", http.StatusFound)
}

func IsAuthenticated(r *http.Request) bool { _, ok := GetCurrentUser(r); return ok }

func GetCurrentUser(r *http.Request) (*User, bool) {
	session, err := sessionStore().Get(r, sessionName)
	if err != nil {
		return nil, false
	}

	username, ok := session.Values[sessionUserKey].(string)
	if !ok || username == "" {
		return nil, false
	}

	u, exists := userStore.GetUser(username)
	if !exists || session.Values["auth_tag"] != authTag(u) {
		return nil, false
	}
	return u, true
}

// authTag invalidates old sessions after password changes without putting the
// password hash itself into a readable signed cookie.
func authTag(u *User) string {
	digest := sha256.Sum256([]byte(u.PasswordHash))
	return hex.EncodeToString(digest[:])
}
