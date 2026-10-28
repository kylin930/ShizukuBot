package plugin

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ============================================================
// 注释标记规范：
//   // shizuku:plugin id=xxx name=xxx version=xxx
//   // shizuku:event event_type title=xxx
//   // shizuku:func name=xxx title=xxx category=xxx desc=xxx
//   // shizuku:input name=xxx type=xxx required=true
//   // shizuku:output name=xxx type=xxx
//   // shizuku:property name=xxx value=xxx
// ============================================================

// ScanResult 代码扫描结果
type ScanResult struct {
	PluginMeta  *PluginMeta       `json:"plugin,omitempty"`
	CustomNodes []CustomNodeDef   `json:"custom_nodes,omitempty"`
	Events      []EventSpec       `json:"events,omitempty"`
	Commands    []CommandSpec     `json:"commands,omitempty"`
	Functions   []FunctionSpec    `json:"functions,omitempty"`
	AllSource   string            `json:"-"` // 所有源码合并（用于 code/go 节点）
}

// PluginMeta 插件元信息（从 shizuku:plugin 注释提取）
type PluginMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`
}

// EventSpec 事件节点规格
type EventSpec struct {
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

// CommandSpec 指令节点规格（从 shizuku:command 注释提取）
type CommandSpec struct {
	Name    string `json:"name"`    // 指令名（不含前缀），如 "ccb"
	Prefix  string `json:"prefix"`  // 前缀，默认 "/"
	Title   string `json:"title"`   // 显示标题
	Handler string `json:"handler"` // 对应的处理函数名
}

// FunctionSpec Go 函数规格（从 shizuku:func 注释提取）
type FunctionSpec struct {
	Name       string            `json:"name"`
	Title      string            `json:"title"`
	Category   string            `json:"category,omitempty"`
	Desc       string            `json:"desc,omitempty"`
	Inputs     []FunctionPort    `json:"inputs,omitempty"`
	Outputs    []FunctionPort    `json:"outputs,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// FunctionPort 函数输入输出端口
type FunctionPort struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
}

// ============================================================
// code2graph: Go 代码扫描器
// ============================================================

type codeScanner struct {
	fset     *token.FileSet
	result   *ScanResult
	curFunc  *FunctionSpec
}

// ScanGoSource 扫描单个 Go 源文件
func ScanGoSource(srcPath string) (*ScanResult, error) {
	scanner := &codeScanner{
		fset:   token.NewFileSet(),
		result: &ScanResult{Functions: make([]FunctionSpec, 0)},
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, err
	}
	return scanner.scanBytes(srcPath, data)
}

// ScanGoDirectory 扫描目录下所有 .go 文件（支持多文件）
func ScanGoDirectory(dirPath string) (*ScanResult, error) {
	scanner := &codeScanner{
		fset:   token.NewFileSet(),
		result: &ScanResult{Functions: make([]FunctionSpec, 0)},
	}
	var allSrc strings.Builder
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		// 保存源码（去掉 package 声明）
		src := string(data)
		if idx := strings.Index(src, "package "); idx >= 0 {
			if nl := strings.Index(src[idx:], "\n"); nl > 0 {
				src = src[idx+nl+1:]
			}
		}
		allSrc.WriteString(src)
		allSrc.WriteString("\n")
		_, scanErr := scanner.scanBytes(path, data)
		return scanErr
	})
	if err != nil {
		return nil, err
	}
	scanner.result.AllSource = allSrc.String()
	return scanner.result, nil
}

// ScanGoBytes 从内存扫描 Go 代码
func ScanGoBytes(data []byte) (*ScanResult, error) {
	scanner := &codeScanner{
		fset:   token.NewFileSet(),
		result: &ScanResult{Functions: make([]FunctionSpec, 0)},
	}
	return scanner.scanBytes("inline.go", data)
}

func (s *codeScanner) scanBytes(filename string, data []byte) (*ScanResult, error) {
	node, err := parser.ParseFile(s.fset, filename, data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %v", filename, err)
	}

	// 扫描文件级注释（shizuku:plugin）
	if node.Comments != nil {
		for _, cg := range node.Comments {
			for _, c := range cg.List {
				s.parseCommentLine(c.Text)
			}
		}
	}

	// 扫描函数声明
	for _, decl := range node.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Doc == nil {
			continue
		}
		s.curFunc = nil
		for _, c := range fd.Doc.List {
			s.parseCommentLine(c.Text)
		}
		if s.curFunc != nil {
			s.result.Functions = append(s.result.Functions, *s.curFunc)
		}
	}

	return s.result, nil
}

var (
	rePlugin  = regexp.MustCompile(`^//\s*shizuku:plugin\s+(.*)$`)
	reEvent   = regexp.MustCompile(`^//\s*shizuku:event\s+(.*)$`)
	reCommand = regexp.MustCompile(`^//\s*shizuku:command\s+(.*)$`)
	reFunc    = regexp.MustCompile(`^//\s*shizuku:func\s+(.*)$`)
	reInput   = regexp.MustCompile(`^//\s*shizuku:input\s+(.*)$`)
	reOutput  = regexp.MustCompile(`^//\s*shizuku:output\s+(.*)$`)
	reProp    = regexp.MustCompile(`^//\s*shizuku:property\s+(.*)$`)
	reKV      = regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"|(\w+)\s*=\s*(\S+)`)
)

// parseCommentLine 解析单行注释
func (s *codeScanner) parseCommentLine(text string) {
	text = strings.TrimSpace(text)

	if m := rePlugin.FindStringSubmatch(text); m != nil {
		s.result.PluginMeta = &PluginMeta{}
		for k, v := range parseKV(m[1]) {
			switch k {
			case "id":
				s.result.PluginMeta.ID = v
			case "name":
				s.result.PluginMeta.Name = v
			case "version":
				s.result.PluginMeta.Version = v
			case "description", "desc":
				s.result.PluginMeta.Description = v
			case "author":
				s.result.PluginMeta.Author = v
			}
		}
		return
	}

	if m := reEvent.FindStringSubmatch(text); m != nil {
		ev := EventSpec{}
		parts := strings.Fields(m[1])
		if len(parts) > 0 && !strings.Contains(parts[0], "=") {
			ev.Type = parts[0]
		}
		for k, v := range parseKV(m[1]) {
			switch k {
			case "title":
				ev.Title = v
			case "type", "event_type":
				ev.Type = v
			}
		}
		s.result.Events = append(s.result.Events, ev)
		return
	}

	if m := reCommand.FindStringSubmatch(text); m != nil {
		cmd := CommandSpec{Prefix: "/"}
		for k, v := range parseKV(m[1]) {
			switch k {
			case "name":
				cmd.Name = v
			case "prefix":
				cmd.Prefix = v
			case "title":
				cmd.Title = v
			case "handler":
				cmd.Handler = v
			}
		}
		if cmd.Title == "" {
			cmd.Title = "收到指令: /" + cmd.Name
		}
		s.result.Commands = append(s.result.Commands, cmd)
		return
	}

	if m := reFunc.FindStringSubmatch(text); m != nil {
		s.curFunc = &FunctionSpec{
			Category:   "自定义",
			Properties: make(map[string]string),
		}
		for k, v := range parseKV(m[1]) {
			switch k {
			case "name":
				s.curFunc.Name = v
			case "title":
				s.curFunc.Title = v
			case "category":
				s.curFunc.Category = v
			case "desc", "description":
				s.curFunc.Desc = v
			}
		}
		return
	}

	if s.curFunc == nil {
		return
	}

	if m := reInput.FindStringSubmatch(text); m != nil {
		port := FunctionPort{Type: "string"}
		for k, v := range parseKV(m[1]) {
			switch k {
			case "name":
				port.Name = v
			case "type":
				port.Type = v
			case "required":
				port.Required, _ = strconv.ParseBool(v)
			}
		}
		if port.Name != "" {
			s.curFunc.Inputs = append(s.curFunc.Inputs, port)
		}
		return
	}

	if m := reOutput.FindStringSubmatch(text); m != nil {
		port := FunctionPort{Type: "string"}
		for k, v := range parseKV(m[1]) {
			switch k {
			case "name":
				port.Name = v
			case "type":
				port.Type = v
			}
		}
		if port.Name != "" {
			s.curFunc.Outputs = append(s.curFunc.Outputs, port)
		}
		return
	}

	if m := reProp.FindStringSubmatch(text); m != nil {
		for k := range parseKV(m[1]) {
			if k != "name" && k != "value" {
				continue
			}
		}
		kv := parseKV(m[1])
		if name, ok := kv["name"]; ok {
			s.curFunc.Properties[name] = kv["value"]
		}
	}
}

// parseKV 解析 key=value 对（支持带引号的值）
func parseKV(s string) map[string]string {
	result := make(map[string]string)
	matches := reKV.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		if m[1] != "" {
			result[m[1]] = m[2]
		} else {
			result[m[3]] = m[4]
		}
	}
	return result
}

// ============================================================
// ScanResult → 插件 JSON（code2graph）
// ============================================================

// ToPluginJSON 把扫描结果转成完整的插件 JSON
func (sr *ScanResult) ToPluginJSON() (map[string]interface{}, error) {
	if sr.PluginMeta == nil {
		sr.PluginMeta = &PluginMeta{
			ID:   "plugin_from_code",
			Name: "从代码生成的插件",
		}
	}

	customNodes := make([]CustomNodeDef, 0, len(sr.Functions))
	for _, fn := range sr.Functions {
		node := fn.ToCustomNodeDef()
		customNodes = append(customNodes, node)
	}

	graph := buildDefaultGraph(sr)

	result := map[string]interface{}{
		"id":           sr.PluginMeta.ID,
		"name":         sr.PluginMeta.Name,
		"version":      sr.PluginMeta.Version,
		"description":  sr.PluginMeta.Description,
		"author":       sr.PluginMeta.Author,
		"enabled":      true,
		"graph":        graph,
		"custom_nodes": customNodes,
	}
	return result, nil
}

// ToCustomNodeDef 把 FunctionSpec 转成自定义节点定义
func (fn *FunctionSpec) ToCustomNodeDef() CustomNodeDef {
	inputs := make([]CustomNodePort, 0, len(fn.Inputs))
	for _, in := range fn.Inputs {
		inputs = append(inputs, CustomNodePort{Name: in.Name, Type: in.Type})
	}

	outputs := make([]CustomNodePort, 0, len(fn.Outputs))
	for _, out := range fn.Outputs {
		outputs = append(outputs, CustomNodePort{Name: out.Name, Type: out.Type})
	}

	// 构建 operations：用 call_func 调用注册的 Go 函数
	args := make(map[string]interface{})
	for _, in := range fn.Inputs {
		args[in.Name] = map[string]interface{}{"input": in.Name}
	}

	ops := []CustomNodeOp{
		{
			Op:   "call_func",
			Func: fn.Name,
			Args: args,
			As:   "__result",
		},
	}

	// 把 call_func 的返回值设到输出端口
	returnOutputs := make(map[string]interface{})
	for _, out := range fn.Outputs {
		returnOutputs[out.Name] = map[string]interface{}{"get": "__result"}
	}
	if len(returnOutputs) > 0 {
		ops = append(ops, CustomNodeOp{
			Op:      "return",
			Outputs: returnOutputs,
		})
	}

	properties := make(map[string]interface{})
	for k, v := range fn.Properties {
		properties[k] = v
	}

	category := fn.Category
	if category == "" {
		category = "自定义"
	}
	title := fn.Title
	if title == "" {
		title = fn.Name
	}

	return CustomNodeDef{
		Type:       fn.Name,
		Title:      title,
		Desc:       fn.Desc,
		Category:   category,
		BoxColor:   "#4a90d9",
		Inputs:     inputs,
		Outputs:    outputs,
		Properties: properties,
		Operations: ops,
	}
}

// buildDefaultGraph 构建默认的流程图
// 优先处理 Commands（event/on_command），没有 Commands 才处理 Events
// 连线格式：[id, origin_id, origin_slot, target_id, target_slot, type]
func buildDefaultGraph(sr *ScanResult) map[string]interface{} {
	nodes := []map[string]interface{}{}
	links := [][]interface{}{}
	nodeID := 1
	linkID := 1
	order := 0

	// 找主处理函数
	var handlerFn *FunctionSpec
	for i := range sr.Functions {
		fn := &sr.Functions[i]
		if fn.Name == "ccb/handle_command" || fn.Name == "ccb/run" {
			handlerFn = fn
			break
		}
	}
	if handlerFn == nil && len(sr.Functions) > 0 {
		handlerFn = &sr.Functions[0]
	}

	// ========== 优先处理 Commands ==========
	if len(sr.Commands) > 0 {
		for cmdIdx, cmd := range sr.Commands {
			eventY := 100 + cmdIdx*220

			// 为每个命令生成完整处理链
			chain := buildCommandChain(cmd, handlerFn, sr.AllSource,
				&nodeID, &linkID, &order, eventY)
			nodes = append(nodes, chain.nodes...)
			links = append(links, chain.links...)
		}

	// ========== 回退到 Events ==========
	} else {
		if len(sr.Events) == 0 {
			sr.Events = []EventSpec{{Type: "event/on_group_message", Title: "收到群消息"}}
		}
		for evIdx, ev := range sr.Events {
			eventY := 100 + evIdx*220
			chain := buildEventChain(ev, handlerFn, sr.AllSource,
				&nodeID, &linkID, &order, eventY)
			nodes = append(nodes, chain.nodes...)
			links = append(links, chain.links...)
		}
	}

	return map[string]interface{}{
		"last_node_id": nodeID - 1,
		"last_link_id": linkID - 1,
		"nodes":         nodes,
		"links":         links,
		"groups":        []interface{}{},
		"config":        map[string]interface{}{},
		"extra":         map[string]interface{}{},
		"version":       0.4,
	}
}

// chainBuildResult 节点链构建结果
type chainBuildResult struct {
	nodes []map[string]interface{}
	links [][]interface{}
}

// buildCommandChain 为单个命令生成处理链
// on_command → code/go → 判断非空 → 发送群消息
func buildCommandChain(cmd CommandSpec, handlerFn *FunctionSpec, allSource string,
	nodeID, linkID, order *int, eventY int) chainBuildResult {

	nodes := []map[string]interface{}{}
	links := [][]interface{}{}

	// ========== 1. on_command 节点 ==========
	eventNodeID := *nodeID
	nodes = append(nodes, map[string]interface{}{
		"id":    eventNodeID,
		"type":  "event/on_command",
		"title": cmd.Title,
		"pos":   []float64{100, float64(eventY)},
		"size":  map[string]interface{}{"0": 210, "1": 340},
		"flags": map[string]interface{}{},
		"order": *order,
		"mode":  0,
		"inputs": []interface{}{},
		"outputs": []map[string]interface{}{
			{"name": "触发", "type": -1, "links": []int{*linkID}, "slot_index": 0},
			{"name": "指令名", "type": "string", "links": nil},
			{"name": "参数列表", "type": "array", "links": nil},
			{"name": "参数文本", "type": "string", "links": nil},
			{"name": "清洗后消息", "type": "string", "links": nil},
			{"name": "原始消息", "type": "string", "links": nil},
			{"name": "发送者ID", "type": "string", "links": nil},
			{"name": "来源ID", "type": "string", "links": nil},
			{"name": "消息ID", "type": "string", "links": nil},
			{"name": "第一个@的ID", "type": "string", "links": nil},
			{"name": "第一个@的昵称", "type": "string", "links": nil},
			{"name": "所有@ID", "type": "array", "links": nil},
			{"name": "所有@昵称", "type": "array", "links": nil},
		},
		"properties": map[string]interface{}{
			"command": cmd.Name,
			"prefix":  cmd.Prefix,
		},
		"boxcolor": "#3a3",
	})
	*nodeID++
	*order++

	if handlerFn == nil {
		return chainBuildResult{nodes: nodes, links: links}
	}

	// ========== 2. code/go 处理节点 ==========
	handlerNodeID := *nodeID
	declarations, bodyCode := buildEmbeddedCodeForCommand(allSource, handlerFn, cmd)

	codeOutputs := []map[string]interface{}{
		{"name": "完成", "type": -1, "links": nil, "slot_index": 0},
	}
	for outIdx, out := range handlerFn.Outputs {
		linksForOut := interface{}(nil)
		if outIdx == 0 {
			linksForOut = []int{*linkID + 1}
		}
		codeOutputs = append(codeOutputs, map[string]interface{}{
			"name":       out.Name,
			"type":       out.Type,
			"links":      linksForOut,
			"slot_index": outIdx + 1,
		})
	}

	nodes = append(nodes, map[string]interface{}{
		"id":    handlerNodeID,
		"type":  "code/go",
		"title": handlerFn.Title,
		"pos":   []float64{420, float64(eventY)},
		"size":  map[string]interface{}{"0": 240, "1": 140},
		"flags": map[string]interface{}{},
		"order": *order,
		"mode":  0,
		"inputs": []map[string]interface{}{
			{"name": "触发", "type": -1, "link": *linkID},
		},
		"outputs": codeOutputs,
		"properties": map[string]interface{}{
			"code":         bodyCode,
			"declarations": declarations,
		},
		"boxcolor": "#4a90d9",
	})
	*nodeID++
	*order++

	// ========== 3. 判断非空节点 ==========
	isEmptyNodeID := *nodeID
	nodes = append(nodes, map[string]interface{}{
		"id":    isEmptyNodeID,
		"type":  "condition/is_empty",
		"title": "响应非空判断",
		"pos":   []float64{700, float64(eventY) - 20},
		"size":  map[string]interface{}{"0": 140, "1": 86},
		"flags": map[string]interface{}{},
		"order": *order,
		"mode":  0,
		"inputs": []map[string]interface{}{
			{"name": "值", "type": "string", "link": *linkID + 1},
		},
		"outputs": []map[string]interface{}{
			{"name": "结果", "type": "boolean", "links": nil, "slot_index": 0},
			{"name": "值", "type": "string", "links": []int{*linkID + 2}, "slot_index": 1},
			{"name": "为空", "type": -1, "links": nil},
			{"name": "不为空", "type": -1, "links": []int{*linkID + 3}, "slot_index": 3},
		},
		"properties": map[string]interface{}{},
		"boxcolor":   "#aa3",
	})
	*nodeID++
	*order++

	// ========== 4. 发送群消息节点 ==========
	sendNodeID := *nodeID
	nodes = append(nodes, map[string]interface{}{
		"id":    sendNodeID,
		"type":  "action/send_group_message",
		"title": "发送群消息",
		"pos":   []float64{980, float64(eventY)},
		"size":  map[string]interface{}{"0": 210, "1": 78},
		"flags": map[string]interface{}{},
		"order": *order,
		"mode":  0,
		"inputs": []map[string]interface{}{
			{"name": "触发", "type": -1, "link": *linkID + 3},
			{"name": "回复内容", "type": "string", "link": *linkID + 2},
		},
		"outputs": []map[string]interface{}{
			{"name": "完成", "type": -1, "links": nil, "slot_index": 0},
		},
		"properties": map[string]interface{}{
			"template": "",
		},
		"boxcolor": "#33a",
	})
	*nodeID++
	*order++

	// ========== 连线 ==========
	// 连线1：on_command 触发 → code/go 触发
	links = append(links, []interface{}{
		*linkID, eventNodeID, 0, handlerNodeID, 0, -1,
	})
	*linkID++

	// 连线2：code/go 响应内容 → 判断 值
	links = append(links, []interface{}{
		*linkID, handlerNodeID, 1, isEmptyNodeID, 0, "string",
	})
	*linkID++

	// 连线3：判断 值 → 发送 回复内容
	links = append(links, []interface{}{
		*linkID, isEmptyNodeID, 1, sendNodeID, 1, "string",
	})
	*linkID++

	// 连线4：判断 不为空 → 发送 触发
	links = append(links, []interface{}{
		*linkID, isEmptyNodeID, 3, sendNodeID, 0, -1,
	})
	*linkID++

	return chainBuildResult{nodes: nodes, links: links}
}

// buildEventChain 为单个事件生成处理链（回退模式）
func buildEventChain(ev EventSpec, handlerFn *FunctionSpec, allSource string,
	nodeID, linkID, order *int, eventY int) chainBuildResult {

	nodes := []map[string]interface{}{}
	links := [][]interface{}{}

	title := ev.Title
	if title == "" {
		title = ev.Type
	}

	// ========== 1. 事件节点 ==========
	eventNodeID := *nodeID
	eventOutputs := []map[string]interface{}{
		{"name": "触发", "type": -1, "links": []int{*linkID}, "slot_index": 0},
		{"name": "完成", "type": -1, "links": nil},
	}
	eventOutputs = appendEventContextPorts(eventOutputs, ev.Type)

	nodes = append(nodes, map[string]interface{}{
		"id":    eventNodeID,
		"type":  ev.Type,
		"title": title,
		"pos":   []float64{100, float64(eventY)},
		"size":  map[string]interface{}{"0": 210, "1": 182},
		"flags": map[string]interface{}{},
		"order": *order,
		"mode":  0,
		"inputs": []interface{}{},
		"outputs": eventOutputs,
		"properties": map[string]interface{}{
			"command": "",
		},
		"boxcolor": "#3a3",
	})
	*nodeID++
	*order++

	if handlerFn == nil {
		return chainBuildResult{nodes: nodes, links: links}
	}

	// ========== 2. code/go 处理节点 ==========
	handlerNodeID := *nodeID
	declarations, bodyCode := buildEmbeddedCode(allSource, handlerFn)

	codeOutputs := []map[string]interface{}{
		{"name": "完成", "type": -1, "links": nil, "slot_index": 0},
	}
	for outIdx, out := range handlerFn.Outputs {
		linksForOut := interface{}(nil)
		if outIdx == 0 {
			linksForOut = []int{*linkID + 1}
		}
		codeOutputs = append(codeOutputs, map[string]interface{}{
			"name":       out.Name,
			"type":       out.Type,
			"links":      linksForOut,
			"slot_index": outIdx + 1,
		})
	}

	nodes = append(nodes, map[string]interface{}{
		"id":    handlerNodeID,
		"type":  "code/go",
		"title": handlerFn.Title,
		"pos":   []float64{420, float64(eventY)},
		"size":  map[string]interface{}{"0": 240, "1": 140},
		"flags": map[string]interface{}{},
		"order": *order,
		"mode":  0,
		"inputs": []map[string]interface{}{
			{"name": "触发", "type": -1, "link": *linkID},
		},
		"outputs": codeOutputs,
		"properties": map[string]interface{}{
			"code":         bodyCode,
			"declarations": declarations,
		},
		"boxcolor": "#4a90d9",
	})
	*nodeID++
	*order++

	// ========== 3. 判断非空节点 ==========
	isEmptyNodeID := *nodeID
	nodes = append(nodes, map[string]interface{}{
		"id":    isEmptyNodeID,
		"type":  "condition/is_empty",
		"title": "响应非空判断",
		"pos":   []float64{700, float64(eventY) - 20},
		"size":  map[string]interface{}{"0": 140, "1": 86},
		"flags": map[string]interface{}{},
		"order": *order,
		"mode":  0,
		"inputs": []map[string]interface{}{
			{"name": "值", "type": "string", "link": *linkID + 1},
		},
		"outputs": []map[string]interface{}{
			{"name": "结果", "type": "boolean", "links": nil, "slot_index": 0},
			{"name": "值", "type": "string", "links": []int{*linkID + 2}, "slot_index": 1},
			{"name": "为空", "type": -1, "links": nil},
			{"name": "不为空", "type": -1, "links": []int{*linkID + 3}, "slot_index": 3},
		},
		"properties": map[string]interface{}{},
		"boxcolor":   "#aa3",
	})
	*nodeID++
	*order++

	// ========== 4. 发送消息节点 ==========
	sendNodeID := *nodeID
	sendNodeType := "action/send_group_message"
	sendNodeTitle := "发送群消息"
	if ev.Type == "event/on_c2c_message" {
		sendNodeType = "action/send_message"
		sendNodeTitle = "发送C2C消息"
	} else if ev.Type == "event/on_guild_message" {
		sendNodeType = "action/send_message"
		sendNodeTitle = "发送频道消息"
	}

	nodes = append(nodes, map[string]interface{}{
		"id":    sendNodeID,
		"type":  sendNodeType,
		"title": sendNodeTitle,
		"pos":   []float64{980, float64(eventY)},
		"size":  map[string]interface{}{"0": 210, "1": 78},
		"flags": map[string]interface{}{},
		"order": *order,
		"mode":  0,
		"inputs": []map[string]interface{}{
			{"name": "触发", "type": -1, "link": *linkID + 3},
			{"name": "回复内容", "type": "string", "link": *linkID + 2},
		},
		"outputs": []map[string]interface{}{
			{"name": "完成", "type": -1, "links": nil, "slot_index": 0},
		},
		"properties": map[string]interface{}{
			"template": "",
		},
		"boxcolor": "#33a",
	})
	*nodeID++
	*order++

	// ========== 连线 ==========
	links = append(links, []interface{}{
		*linkID, eventNodeID, 0, handlerNodeID, 0, -1,
	})
	*linkID++

	links = append(links, []interface{}{
		*linkID, handlerNodeID, 1, isEmptyNodeID, 0, "string",
	})
	*linkID++

	links = append(links, []interface{}{
		*linkID, isEmptyNodeID, 1, sendNodeID, 1, "string",
	})
	*linkID++

	links = append(links, []interface{}{
		*linkID, isEmptyNodeID, 3, sendNodeID, 0, -1,
	})
	*linkID++

	return chainBuildResult{nodes: nodes, links: links}
}

// appendEventContextPorts 为事件节点追加上下文输出端口
func appendEventContextPorts(outputs []map[string]interface{}, eventType string) []map[string]interface{} {
	commonPorts := []map[string]interface{}{
		{"name": "发送者ID", "type": "string", "links": nil},
		{"name": "发送者昵称", "type": "string", "links": nil},
		{"name": "消息ID", "type": "string", "links": nil},
		{"name": "所有@ID", "type": "array", "links": nil},
		{"name": "所有@昵称", "type": "array", "links": nil},
	}
	switch eventType {
	case "event/on_group_message":
		outputs = append(outputs, map[string]interface{}{"name": "群聊ID", "type": "string", "links": nil})
	case "event/on_c2c_message":
		// C2C 没有群聊ID
	case "event/on_guild_message":
		outputs = append(outputs,
			map[string]interface{}{"name": "频道ID", "type": "string", "links": nil},
			map[string]interface{}{"name": "GuildID", "type": "string", "links": nil},
		)
	}
	outputs = append(outputs, commonPorts...)
	return outputs
}

// buildEmbeddedCode 构建嵌入 code/go 节点的代码
// 返回 declarations（包级声明：类型、变量、函数）和 body（函数体内部执行代码）
func buildEmbeddedCode(allSource string, handlerFn *FunctionSpec) (declarations string, body string) {
	// 去掉 import（code_executor 已经注入了）
	src := stripImportBlock(allSource)

	var bodySb strings.Builder

	// 调用入口函数（从 sctx 获取参数）
	bodySb.WriteString("// ===== 入口调用 =====\n")
	bodySb.WriteString("var _handler_result = HandleCommand(message, userID, groupID)\n")

	// 设置 output（把函数返回值映射到输出端口）
	if len(handlerFn.Outputs) > 0 {
		bodySb.WriteString(fmt.Sprintf("output[%q] = _handler_result\n", handlerFn.Outputs[0].Name))
	}
	for i := 1; i < len(handlerFn.Outputs); i++ {
		bodySb.WriteString(fmt.Sprintf("// output[%q] = ...\n", handlerFn.Outputs[i].Name))
	}

	return src, bodySb.String()
}

// buildEmbeddedCodeForCommand 为命令节点构建嵌入代码
// 和 buildEmbeddedCode 类似，用 sctx 的 Message/UserID/GroupID 调用
func buildEmbeddedCodeForCommand(allSource string, handlerFn *FunctionSpec, cmd CommandSpec) (declarations string, body string) {
	src := stripImportBlock(allSource)

	var bodySb strings.Builder
	bodySb.WriteString("// ===== 入口调用 =====\n")
	bodySb.WriteString(fmt.Sprintf("// 命令: %s%s\n", cmd.Prefix, cmd.Name))
	bodySb.WriteString("var _handler_result = HandleCommand(message, userID, groupID)\n")

	if len(handlerFn.Outputs) > 0 {
		bodySb.WriteString(fmt.Sprintf("output[%q] = _handler_result\n", handlerFn.Outputs[0].Name))
	}
	for i := 1; i < len(handlerFn.Outputs); i++ {
		bodySb.WriteString(fmt.Sprintf("// output[%q] = ...\n", handlerFn.Outputs[i].Name))
	}

	return src, bodySb.String()
}

// stripImportBlock 去掉源码中的 import 块（避免和 code_executor 注入的 import 冲突）
func stripImportBlock(src string) string {
	// 去掉单行 import
	src = regexp.MustCompile(`(?m)^import\s+"[^"]+"\s*$`).ReplaceAllString(src, "")
	// 去掉多行 import 块
	src = regexp.MustCompile(`(?s)import\s*\([^)]*\)`).ReplaceAllString(src, "")
	return src
}

// ============================================================
// graph2code: 流程图 → Go 代码骨架
// ============================================================

// GraphToCode 把流程图 JSON 转成 Go 代码骨架
func GraphToCode(pluginJSON map[string]interface{}) (string, error) {
	graphData, _ := pluginJSON["graph"].(map[string]interface{})
	if graphData == nil {
		return "", fmt.Errorf("no graph data")
	}

	nodesRaw, _ := graphData["nodes"].([]interface{})
	pluginName, _ := pluginJSON["name"].(string)
	pluginID, _ := pluginJSON["id"].(string)

	var sb strings.Builder
	sb.WriteString("// Code generated by ShizukuBot graph2code. DO NOT EDIT.\n")
	sb.WriteString(fmt.Sprintf("// Plugin: %s (ID: %s)\n", pluginName, pluginID))
	sb.WriteString("package main\n\n")
	sb.WriteString("import (\n")
	sb.WriteString("\t\"fmt\"\n")
	sb.WriteString("\t\"shizukubot/plugin\"\n")
	sb.WriteString(")\n\n")

	sb.WriteString(fmt.Sprintf("// %s 插件入口函数\n", pluginName))
	sb.WriteString(fmt.Sprintf("func %s(ctx *plugin.EventContext) {\n", safeFuncName(pluginName)))

	// 为每个节点生成变量和调用
	for i, nRaw := range nodesRaw {
		node, _ := nRaw.(map[string]interface{})
		if node == nil {
			continue
		}
		nodeType, _ := node["type"].(string)
		title, _ := node["title"].(string)
		props, _ := node["properties"].(map[string]interface{})

		sb.WriteString(fmt.Sprintf("\n\t// 节点[%d]: %s (type=%s)\n", i, title, nodeType))

		switch nodeType {
		case "event/on_group_message", "event/on_c2c_message", "event/on_guild_message":
			sb.WriteString(fmt.Sprintf("\t// 事件触发: %s\n", title))
			sb.WriteString("\t_ = ctx.Message  // 消息内容\n")
			sb.WriteString("\t_ = ctx.UserID   // 发送者ID\n")
			sb.WriteString("\t_ = ctx.GroupID  // 群聊ID\n")

		case "action/send_group_message", "action/send_message":
			content := ""
			if props != nil {
				if c, ok := props["content"].(string); ok {
					content = c
				}
			}
			sb.WriteString(fmt.Sprintf("\terr := ctx.ReplyToGroup(%q)\n", content))
			sb.WriteString("\tif err != nil {\n")
			sb.WriteString("\t\tfmt.Println(\"发送失败:\", err)\n")
			sb.WriteString("\t}\n")

		case "condition/number_compare":
			sb.WriteString("\t// 数字比较: 请在此处填写比较逻辑\n")
			sb.WriteString("\tvar valueA, valueB float64\n")
			sb.WriteString("\tvar operator = \">=\" // ==, !=, >, >=, <, <=\n")
			sb.WriteString("\tvar conditionResult = compareNumbers(valueA, valueB, operator)\n")
			sb.WriteString("\tif conditionResult {\n")
			sb.WriteString("\t\t// 条件成立分支\n")
			sb.WriteString("\t} else {\n")
			sb.WriteString("\t\t// 条件不成立分支\n")
			sb.WriteString("\t}\n")

		case "logic/branch":
			sb.WriteString("\t// 条件分支\n")
			sb.WriteString("\tvar branchCondition bool\n")
			sb.WriteString("\tif branchCondition {\n")
			sb.WriteString("\t\t// 条件成立\n")
			sb.WriteString("\t} else {\n")
			sb.WriteString("\t\t// 条件不成立\n")
			sb.WriteString("\t}\n")

		case "variable/set":
			sb.WriteString("\t// 设置变量\n")
			varName := "myVar"
			if props != nil {
				if n, ok := props["name"].(string); ok {
					varName = n
				}
			}
			sb.WriteString(fmt.Sprintf("\tvar %s interface{}\n", varName))

		default:
			sb.WriteString(fmt.Sprintf("\t// TODO: 实现节点 %s 的逻辑\n", nodeType))
			propsJSON, _ := json.Marshal(props)
			sb.WriteString(fmt.Sprintf("\t// 属性: %s\n", string(propsJSON)))
		}
	}

	sb.WriteString("}\n\n")

	// 辅助函数
	sb.WriteString("// compareNumbers 数字比较辅助函数\n")
	sb.WriteString("func compareNumbers(a, b float64, op string) bool {\n")
	sb.WriteString("\tswitch op {\n")
	sb.WriteString("\tcase \"==\": return a == b\n")
	sb.WriteString("\tcase \"!=\": return a != b\n")
	sb.WriteString("\tcase \">\":  return a > b\n")
	sb.WriteString("\tcase \">=\": return a >= b\n")
	sb.WriteString("\tcase \"<\":  return a < b\n")
	sb.WriteString("\tcase \"<=\": return a <= b\n")
	sb.WriteString("\tdefault: return false\n")
	sb.WriteString("\t}\n")
	sb.WriteString("}\n")

	return sb.String(), nil
}

func safeFuncName(s string) string {
	sb := strings.Builder{}
	for i, r := range s {
		if i == 0 {
			if r >= '0' && r <= '9' {
				sb.WriteRune('_')
			}
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	result := sb.String()
	if result == "" {
		return "pluginFunc"
	}
	return result
}
