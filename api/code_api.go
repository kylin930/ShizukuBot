package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"shizukubot/plugin"
)

// ============================================================
// 代码转换相关 API
// ============================================================

// ScanCodeRequest 扫描 Go 代码请求
type ScanCodeRequest struct {
	SourceType string `json:"source_type"` // "file", "directory", "inline"
	Path       string `json:"path"`        // 文件或目录路径（source_type=file/directory 时）
	Code       string `json:"code"`        // 内联代码（source_type=inline 时）
}

// CodeToGraphRequest Go 代码 → 插件 JSON
type CodeToGraphRequest struct {
	SourceType string `json:"source_type"`
	Path       string `json:"path"`
	Code       string `json:"code"`
}

// GraphToCodeRequest 插件 JSON → Go 代码
type GraphToCodeRequest struct {
	Plugin map[string]interface{} `json:"plugin"`
}

// ScanCode 扫描 Go 代码，返回提取的元信息和自定义节点
func (r *Router) ScanCode(w http.ResponseWriter, req *http.Request) {
	var body ScanCodeRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		ErrorResponse(w, 400, "invalid request body: "+err.Error())
		return
	}

	var result *plugin.ScanResult
	var err error

	switch body.SourceType {
	case "inline":
		result, err = plugin.ScanGoBytes([]byte(body.Code))
	case "file":
		if body.Path == "" {
			ErrorResponse(w, 400, "path is required for source_type=file")
			return
		}
		result, err = plugin.ScanGoSource(body.Path)
	case "directory":
		if body.Path == "" {
			ErrorResponse(w, 400, "path is required for source_type=directory")
			return
		}
		absPath, _ := filepath.Abs(body.Path)
		if _, statErr := os.Stat(absPath); statErr != nil {
			ErrorResponse(w, 400, "directory not found: "+absPath)
			return
		}
		result, err = plugin.ScanGoDirectory(absPath)
	default:
		ErrorResponse(w, 400, "unknown source_type: "+body.SourceType)
		return
	}

	if err != nil {
		ErrorResponse(w, 500, "scan failed: "+err.Error())
		return
	}

	SuccessResponse(w, result)
}

// CodeToGraph Go 代码 → 完整插件 JSON
func (r *Router) CodeToGraph(w http.ResponseWriter, req *http.Request) {
	var body CodeToGraphRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		ErrorResponse(w, 400, "invalid request body: "+err.Error())
		return
	}

	var scanResult *plugin.ScanResult
	var err error

	switch body.SourceType {
	case "inline":
		scanResult, err = plugin.ScanGoBytes([]byte(body.Code))
	case "file":
		if body.Path == "" {
			ErrorResponse(w, 400, "path is required")
			return
		}
		scanResult, err = plugin.ScanGoSource(body.Path)
	case "directory":
		if body.Path == "" {
			ErrorResponse(w, 400, "path is required")
			return
		}
		scanResult, err = plugin.ScanGoDirectory(body.Path)
	default:
		ErrorResponse(w, 400, "unknown source_type: "+body.SourceType)
		return
	}

	if err != nil {
		ErrorResponse(w, 500, "scan failed: "+err.Error())
		return
	}

	pluginJSON, err := scanResult.ToPluginJSON()
	if err != nil {
		ErrorResponse(w, 500, "generate plugin failed: "+err.Error())
		return
	}

	SuccessResponse(w, pluginJSON)
}

// GraphToCode 插件 JSON → Go 代码骨架
func (r *Router) GraphToCode(w http.ResponseWriter, req *http.Request) {
	var body GraphToCodeRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		ErrorResponse(w, 400, "invalid request body: "+err.Error())
		return
	}

	if body.Plugin == nil {
		ErrorResponse(w, 400, "plugin is required")
		return
	}

	code, err := plugin.GraphToCode(body.Plugin)
	if err != nil {
		ErrorResponse(w, 500, "generate code failed: "+err.Error())
		return
	}

	SuccessResponse(w, map[string]interface{}{
		"code": code,
	})
}
