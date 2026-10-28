package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"shizukubot/webui"
)

func (r *Router) GetUsers(w http.ResponseWriter, req *http.Request) {
	user, ok := webui.GetCurrentUser(req)
	if !ok || !user.IsAdmin {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "无权限访问",
		})
		return
	}

	users := webui.GetUserStore().GetAllUsers()
	result := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		result = append(result, map[string]interface{}{
			"username":   u.Username,
			"is_admin":   u.IsAdmin,
			"created_at": u.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 0,
		"data": result,
	})
}

func (r *Router) CreateUser(w http.ResponseWriter, req *http.Request) {
	user, ok := webui.GetCurrentUser(req)
	if !ok || !user.IsAdmin {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "无权限访问",
		})
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "请求格式错误",
		})
		return
	}

	if body.Username == "" || body.Password == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "用户名和密码不能为空",
		})
		return
	}

	if err := webui.GetUserStore().CreateUser(body.Username, body.Password, body.IsAdmin); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "创建成功",
	})
}

func (r *Router) DeleteUser(w http.ResponseWriter, req *http.Request) {
	user, ok := webui.GetCurrentUser(req)
	if !ok || !user.IsAdmin {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "无权限访问",
		})
		return
	}

	vars := mux.Vars(req)
	username := vars["username"]

	if err := webui.GetUserStore().DeleteUser(username); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "删除成功",
	})
}

func (r *Router) ChangeUserPassword(w http.ResponseWriter, req *http.Request) {
	currentUser, ok := webui.GetCurrentUser(req)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "未登录",
		})
		return
	}

	vars := mux.Vars(req)
	username := vars["username"]

	if username != currentUser.Username && !currentUser.IsAdmin {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "无权限修改他人密码",
		})
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "请求格式错误",
		})
		return
	}

	if body.Password == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "密码不能为空",
		})
		return
	}

	if err := webui.GetUserStore().ChangePassword(username, body.Password); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "密码修改成功",
	})
}

func (r *Router) GetCurrentUser(w http.ResponseWriter, req *http.Request) {
	user, ok := webui.GetCurrentUser(req)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "未登录",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"username": user.Username,
			"is_admin": user.IsAdmin,
		},
	})
}
