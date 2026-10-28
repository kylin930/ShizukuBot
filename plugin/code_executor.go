package plugin

import (
	"encoding/json"
	"fmt"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// ============================================================
// code/go 节点：在流程图里直接写 Go 代码
// ============================================================

// CodeGoExecutor 执行内嵌 Go 代码的节点
type CodeGoExecutor struct{}

// yaegi 每次执行创建新实例（避免重复声明问题）
func newYaegi() *interp.Interpreter {
	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)
	return i
}

func (e *CodeGoExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	code := getStrProp(node, "code")
	declarations := getStrProp(node, "declarations")
	if code == "" && declarations == "" {
		return nil, fmt.Errorf("code/go: both code and declarations are empty")
	}

	// 注入可用的上下文变量
	inputsJSON, _ := json.Marshal(inputs)
	ctxData := map[string]interface{}{
		"BotID":     ctx.BotID,
		"EventType": ctx.EventType,
		"Message":   ctx.Message,
		"UserID":    ctx.UserID,
		"GroupID":   ctx.GroupID,
		"ChannelID": ctx.ChannelID,
		"GuildID":   ctx.GuildID,
		"MessageID": ctx.MessageID,
		"Inputs":    inputs,
	}
	ctxJSON, _ := json.Marshal(ctxData)

	// 合并成一次 Eval：declarations（包级声明） + run() 函数（执行体）
	// 注意：每次执行用新的 yaegi 实例，避免重复声明
	fullCode := fmt.Sprintf(`
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

%s

func run() map[string]interface{} {
	var inputs map[string]interface{}
	json.Unmarshal([]byte(%q), &inputs)

	var sctx map[string]interface{}
	json.Unmarshal([]byte(%q), &sctx)

	var output = make(map[string]interface{})

	// 便捷变量
	message, _ := sctx["Message"].(string)
	userID, _ := sctx["UserID"].(string)
	groupID, _ := sctx["GroupID"].(string)
	eventType, _ := sctx["EventType"].(string)

	_ = message
	_ = userID
	_ = groupID
	_ = eventType
	_ = inputs
	_ = sctx
	_ = fmt.Sprintf
	_ = strconv.Itoa
	_ = strings.TrimSpace
	_ = rand.Intn
	_ = math.Round

	%s

	return output
}
`, declarations, string(inputsJSON), string(ctxJSON), code)

	i := newYaegi()
	_, err := i.Eval(fullCode)
	if err != nil {
		return nil, fmt.Errorf("code/go eval error: %v", err)
	}

	// 调用 run 函数获取结果
	v, err := i.Eval("run()")
	if err != nil {
		return nil, fmt.Errorf("code/go run error: %v", err)
	}

	output := make(map[string]interface{})
	if !v.IsNil() && v.IsValid() {
		if m, ok := v.Interface().(map[string]interface{}); ok {
			output = m
		}
	}

	return output, nil
}
