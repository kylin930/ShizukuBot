package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

// ==================== CORS ====================

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ==================== 参数校验常量 ====================

const (
	maxNameLen        = 50
	maxDescLen        = 500
	maxAuthorLen      = 50
	maxVersionLen     = 20
	maxDownloadURLLen = 500
	maxKeywordLen     = 20
	maxKeywordsCount  = 10
)

func SetupRouter() *mux.Router {
	r := mux.NewRouter()

	r.Use(corsMiddleware)

	// gorilla/mux 的 MethodNotAllowedHandler / NotFoundHandler 不经过 r.Use() 中间件
	// 需要单独设置，确保 OPTIONS 预检和 405/404 响应也带 CORS 头
	corsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	r.MethodNotAllowedHandler = corsHandler
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	r.HandleFunc("/api/config", getConfigHandler).Methods(http.MethodGet)
	r.HandleFunc("/api/sts", getSTSHandler).Methods(http.MethodGet)
	r.HandleFunc("/api/plugins", addPluginHandler).Methods(http.MethodPost)
	r.HandleFunc("/api/plugins/{id}/download", downloadPluginHandler).Methods(http.MethodPost)
	r.HandleFunc("/api/stats", getStatsHandler).Methods(http.MethodGet)
	r.HandleFunc("/api/plugins/search", searchPluginsHandler).Methods(http.MethodGet)

	return r
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func successResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    data,
	})
}

func errorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    code,
		"message": message,
	})
}

// getConfigHandler 返回客户端需要的配置（OSS 地址等）
func getConfigHandler(w http.ResponseWriter, req *http.Request) {
	successResponse(w, map[string]interface{}{
		"oss_base":   config.OSS.PublicBase,
		"page_size":  50,
		"volume_max": 300,
	})
}

func getSTSHandler(w http.ResponseWriter, req *http.Request) {
	if config.OSS.STSAPI == "" {
		errorResponse(w, http.StatusNotImplemented, "STS API not configured")
		return
	}

	token, err := authMgr.GetToken()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "获取认证token失败: "+err.Error())
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	r, err := http.NewRequest("GET", config.OSS.STSAPI, nil)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	r.Header.Set("Authorization", token)

	resp, err := client.Do(r)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer resp.Body.Close()

	var stsResult map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stsResult); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	successResponse(w, map[string]interface{}{
		"AccessKeyId":     stsResult["AccessKeyId"],
		"AccessKeySecret": stsResult["AccessKeySecret"],
		"SecurityToken":   stsResult["SecurityToken"],
		"Expiration":      stsResult["Expiration"],
		"region":          config.OSS.Region,
		"endpoint":        config.OSS.Endpoint,
		"bucket_name":     config.OSS.BucketName,
		"bucket":          config.OSS.BucketName,
		"public_base":     config.OSS.PublicBase,
	})
}

func addPluginHandler(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		PluginInfo
		Password string `json:"password"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		errorResponse(w, http.StatusBadRequest, "请求体格式错误")
		return
	}

	p := &payload.PluginInfo

	if len(p.Name) > maxNameLen {
		errorResponse(w, http.StatusBadRequest, "名称不能超过"+strconv.Itoa(maxNameLen)+"个字符")
		return
	}
	if len(p.Description) > maxDescLen {
		errorResponse(w, http.StatusBadRequest, "描述不能超过"+strconv.Itoa(maxDescLen)+"个字符")
		return
	}
	if len(p.Author) > maxAuthorLen {
		errorResponse(w, http.StatusBadRequest, "作者不能超过"+strconv.Itoa(maxAuthorLen)+"个字符")
		return
	}
	if len(p.Version) > maxVersionLen {
		errorResponse(w, http.StatusBadRequest, "版本号不能超过"+strconv.Itoa(maxVersionLen)+"个字符")
		return
	}
	if len(p.DownloadURL) > maxDownloadURLLen {
		errorResponse(w, http.StatusBadRequest, "下载链接不能超过"+strconv.Itoa(maxDownloadURLLen)+"个字符")
		return
	}
	if len(p.Keywords) > maxKeywordsCount {
		errorResponse(w, http.StatusBadRequest, "关键词最多"+strconv.Itoa(maxKeywordsCount)+"个")
		return
	}
	for _, kw := range p.Keywords {
		if len(kw) > maxKeywordLen {
			errorResponse(w, http.StatusBadRequest, "单个关键词不能超过"+strconv.Itoa(maxKeywordLen)+"个字符")
			return
		}
	}

	if p.Name == "" {
		errorResponse(w, http.StatusBadRequest, "名称不能为空")
		return
	}
	if p.Author == "" {
		errorResponse(w, http.StatusBadRequest, "作者不能为空")
		return
	}
	if p.DownloadURL == "" {
		errorResponse(w, http.StatusBadRequest, "下载链接不能为空")
		return
	}
	if p.Category == "" {
		errorResponse(w, http.StatusBadRequest, "分类不能为空")
		return
	}

	if err := verifyPluginPassword(p.Author, payload.Password); err != nil {
		errorResponse(w, http.StatusForbidden, err.Error())
		return
	}
	addPendingPlugin(p, payload.Password)

	successResponse(w, map[string]interface{}{
		"id":      p.ID,
		"message": "插件已加入队列，将在15分钟内同步到OSS",
	})
}

func downloadPluginHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	clientIP := req.RemoteAddr
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		clientIP = xff
	}

	if !increaseDownloads(clientIP, id) {
		errorResponse(w, http.StatusTooManyRequests, "下载过于频繁，请稍后重试")
		return
	}

	successResponse(w, map[string]interface{}{
		"id":        id,
		"downloads": getDownloadCount(id),
	})
}

func getStatsHandler(w http.ResponseWriter, req *http.Request) {
	successResponse(w, getStats())
}

func searchPluginsHandler(w http.ResponseWriter, req *http.Request) {
	query := req.URL.Query().Get("q")
	limitStr := req.URL.Query().Get("limit")
	limit := 20
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	if query == "" {
		successResponse(w, []interface{}{})
		return
	}

	results, err := getSearchSuggestions(query)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	if len(results) > limit {
		results = results[:limit]
	}

	successResponse(w, results)
}
