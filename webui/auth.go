package webui

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/sessions"
)

const (
	sessionName    = "shizukubot_session"
	sessionUserKey = "username"
)

var store *sessions.CookieStore

func InitSessionStore(secret string) {
	store = sessions.NewCookieStore([]byte(secret))
	store.Options = &sessions.Options{
		HttpOnly: true,
		Secure:   false,
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

	session, err := store.Get(r, sessionName)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "服务器内部错误",
		})
		return
	}

	session.Values[sessionUserKey] = user.Username
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
	session, err := store.Get(r, sessionName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	session.Values[sessionUserKey] = nil
	session.Options.MaxAge = -1
	if err := session.Save(r, w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/login", http.StatusFound)
}

func IsAuthenticated(r *http.Request) bool {
	session, err := store.Get(r, sessionName)
	if err != nil {
		return false
	}

	username, ok := session.Values[sessionUserKey].(string)
	if !ok || username == "" {
		return false
	}

	_, exists := userStore.GetUser(username)
	return exists
}

func GetCurrentUser(r *http.Request) (*User, bool) {
	session, err := store.Get(r, sessionName)
	if err != nil {
		return nil, false
	}

	username, ok := session.Values[sessionUserKey].(string)
	if !ok || username == "" {
		return nil, false
	}

	return userStore.GetUser(username)
}