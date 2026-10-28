package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// CustomNodeDef 自定义节点定义
type CustomNodeDef struct {
	Type       string                 `json:"type"`
	Title      string                 `json:"title"`
	Desc       string                 `json:"desc"`
	Category   string                 `json:"category"`
	BoxColor   string                 `json:"boxcolor"`
	Inputs     []CustomNodePort       `json:"inputs"`
	Outputs    []CustomNodePort       `json:"outputs"`
	Properties map[string]interface{} `json:"properties"`
	Operations []CustomNodeOp         `json:"operations"`
}

// CustomNodePort 端口定义
type CustomNodePort struct {
	Name string      `json:"name"`
	Type interface{} `json:"type"` // string: "string"/"number"/"boolean"/"array", number: -1(EVENT/ACTION)
}

// CustomNodeOp 操作步骤
type CustomNodeOp struct {
	Op      string                 `json:"op"`
	File    string                 `json:"file,omitempty"`
	As      string                 `json:"as,omitempty"`
	Default interface{}            `json:"default,omitempty"`
	Name    string                 `json:"name,omitempty"`
	Value   interface{}            `json:"value,omitempty"`
	From    string                 `json:"from,omitempty"`
	To      string                 `json:"to,omitempty"`
	Path    []string               `json:"path,omitempty"`
	Outputs map[string]interface{} `json:"outputs,omitempty"`
	Target  string                 `json:"target,omitempty"`
	Func    string                 `json:"func,omitempty"`
	Args    interface{}            `json:"args,omitempty"`
}

// ==================== Go 函数白名单注册表 ====================

// CustomGoFunc 自定义节点可调用的 Go 函数签名
type CustomGoFunc func(ctx *EventContext, args map[string]interface{}) (interface{}, error)

var customGoFuncRegistry = make(map[string]CustomGoFunc)
var customGoFuncMu sync.RWMutex

// RegisterCustomGoFunc 注册一个可供自定义节点调用的 Go 函数
func RegisterCustomGoFunc(name string, fn CustomGoFunc) {
	customGoFuncMu.Lock()
	defer customGoFuncMu.Unlock()
	customGoFuncRegistry[name] = fn
}

func getCustomGoFunc(name string) (CustomGoFunc, bool) {
	customGoFuncMu.RLock()
	defer customGoFuncMu.RUnlock()
	fn, ok := customGoFuncRegistry[name]
	return fn, ok
}

func init() {
	// ==================== 内置可用 Go 函数 ====================

	// file/read_file: 读取文件内容为字符串
	RegisterCustomGoFunc("file/read_file", func(ctx *EventContext, args map[string]interface{}) (interface{}, error) {
		path := fmt.Sprintf("%v", args["path"])
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return string(data), nil
	})

	// file/write_file: 写入字符串到文件
	RegisterCustomGoFunc("file/write_file", func(ctx *EventContext, args map[string]interface{}) (interface{}, error) {
		path := fmt.Sprintf("%v", args["path"])
		content := fmt.Sprintf("%v", args["content"])
		return nil, os.WriteFile(path, []byte(content), 0644)
	})

	// file/append_file: 追加字符串到文件
	RegisterCustomGoFunc("file/append_file", func(ctx *EventContext, args map[string]interface{}) (interface{}, error) {
		path := fmt.Sprintf("%v", args["path"])
		content := fmt.Sprintf("%v", args["content"])
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		_, err = f.WriteString(content)
		return nil, err
	})

	// file/exists: 检查文件是否存在
	RegisterCustomGoFunc("file/exists", func(ctx *EventContext, args map[string]interface{}) (interface{}, error) {
		path := fmt.Sprintf("%v", args["path"])
		_, err := os.Stat(path)
		return err == nil, nil
	})

	// text/format: 格式化字符串（支持 %s %d %v 占位符）
	RegisterCustomGoFunc("text/format", func(ctx *EventContext, args map[string]interface{}) (interface{}, error) {
		tpl := fmt.Sprintf("%v", args["template"])
		if params, ok := args["params"].([]interface{}); ok {
			ifaces := make([]interface{}, len(params))
			for i, p := range params {
				ifaces[i] = p
			}
			return fmt.Sprintf(tpl, ifaces...), nil
		}
		return tpl, nil
	})

	// text/join: 用分隔符连接列表
	RegisterCustomGoFunc("text/join", func(ctx *EventContext, args map[string]interface{}) (interface{}, error) {
		sep := fmt.Sprintf("%v", args["sep"])
		if arr, ok := args["items"].([]interface{}); ok {
			parts := make([]string, len(arr))
			for i, v := range arr {
				parts[i] = fmt.Sprintf("%v", v)
			}
			return strings.Join(parts, sep), nil
		}
		return "", nil
	})

	// list/length: 列表长度
	RegisterCustomGoFunc("list/length", func(ctx *EventContext, args map[string]interface{}) (interface{}, error) {
		if arr, ok := args["list"].([]interface{}); ok {
			return float64(len(arr)), nil
		}
		return float64(0), nil
	})

	// math/eval: 执行数学计算（支持 + - * / ()）
	RegisterCustomGoFunc("math/eval", func(ctx *EventContext, args map[string]interface{}) (interface{}, error) {
		expr := fmt.Sprintf("%v", args["expr"])
		return safeEvalMath(expr)
	})
}

// opValueRef 操作值引用（从 input/property/ctx 获取）
type opValueRef struct {
	Input    string      `json:"input,omitempty"`
	Property string      `json:"property,omitempty"`
	Fallback interface{} `json:"fallback,omitempty"`
	Expr     string      `json:"expr,omitempty"`
	Get      string      `json:"get,omitempty"`
}

// CustomNodeStore 自定义节点存储
type CustomNodeStore struct {
	dir    string
	nodes  map[string]*CustomNodeDef
	mu     sync.RWMutex
}

// NewCustomNodeStore 创建自定义节点存储
func NewCustomNodeStore(dir string) (*CustomNodeStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	store := &CustomNodeStore{
		dir:   dir,
		nodes: make(map[string]*CustomNodeDef),
	}
	if err := store.Load(); err != nil {
		return nil, err
	}
	return store, nil
}

// Load 从目录加载所有自定义节点
func (s *CustomNodeStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}

	s.nodes = make(map[string]*CustomNodeDef)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			fmt.Printf("[CustomNode] failed to read %s: %v\n", entry.Name(), err)
			continue
		}
		var def CustomNodeDef
		if err := json.Unmarshal(data, &def); err != nil {
			fmt.Printf("[CustomNode] failed to parse %s: %v\n", entry.Name(), err)
			continue
		}
		if def.Type == "" {
			fmt.Printf("[CustomNode] skip %s: empty type\n", entry.Name())
			continue
		}
		s.nodes[def.Type] = &def
	}
	return nil
}

// List 获取所有自定义节点定义（供前端使用）
func (s *CustomNodeStore) List() []*CustomNodeDef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*CustomNodeDef, 0, len(s.nodes))
	for _, def := range s.nodes {
		result = append(result, def)
	}
	return result
}

// Get 获取指定类型的自定义节点定义
func (s *CustomNodeStore) Get(nodeType string) (*CustomNodeDef, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	def, ok := s.nodes[nodeType]
	return def, ok
}

// RegisterAll 注册所有自定义节点到执行引擎
func (s *CustomNodeStore) RegisterAll(engine *ExecutionEngine) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, def := range s.nodes {
		engine.RegisterExecutor(def.Type, &CustomNodeExecutor{def: def})
	}
}

// ==================== 自定义节点执行器 ====================

// CustomNodeExecutor 自定义节点执行器
type CustomNodeExecutor struct {
	def *CustomNodeDef
}

func (e *CustomNodeExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	vm := &customNodeVM{
		ctx:      ctx,
		node:     node,
		inputs:   inputs,
		variables: make(map[string]interface{}),
		outputs:  make(map[string]interface{}),
	}
	// 注入 ctx 字段到变量空间
	vm.variables["ctx.GroupID"] = ctx.GroupID
	vm.variables["ctx.UserID"] = ctx.UserID
	vm.variables["ctx.MessageID"] = ctx.MessageID
	vm.variables["ctx.Message"] = ctx.Message
	vm.variables["ctx.ChannelID"] = ctx.ChannelID
	vm.variables["ctx.GuildID"] = ctx.GuildID
	vm.variables["ctx.BotID"] = ctx.BotID

	if err := vm.run(e.def.Operations); err != nil {
		return nil, err
	}
	return vm.outputs, nil
}

// customNodeVM 自定义节点虚拟机
type customNodeVM struct {
	ctx       *EventContext
	node      *NodeData
	inputs    map[string]interface{}
	variables map[string]interface{}
	outputs   map[string]interface{}
}

func (vm *customNodeVM) run(ops []CustomNodeOp) error {
	for _, op := range ops {
		if err := vm.execOp(op); err != nil {
			return err
		}
	}
	return nil
}

func (vm *customNodeVM) execOp(op CustomNodeOp) error {
	switch op.Op {
	case "load_json":
		return vm.opLoadJSON(op)
	case "save_json":
		return vm.opSaveJSON(op)
	case "set_var":
		return vm.opSetVar(op)
	case "get_path":
		return vm.opGetPath(op)
	case "set_path":
		return vm.opSetPath(op)
	case "call_func":
		return vm.opCallFunc(op)
	case "return":
		return vm.opReturn(op)
	default:
		return fmt.Errorf("unknown op: %s", op.Op)
	}
}

func (vm *customNodeVM) opLoadJSON(op CustomNodeOp) error {
	file := vm.resolvePath(op.File)
	data, err := os.ReadFile(file)
	if err != nil {
		vm.variables[op.As] = cloneValue(op.Default)
		return nil
	}
	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		vm.variables[op.As] = cloneValue(op.Default)
		return nil
	}
	vm.variables[op.As] = result
	return nil
}

func (vm *customNodeVM) opSaveJSON(op CustomNodeOp) error {
	file := vm.resolvePath(op.File)
	val := vm.resolveValue(op.Value)
	data, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0644)
}

func (vm *customNodeVM) opSetVar(op CustomNodeOp) error {
	val := vm.resolveValue(op.Value)
	vm.setVar(op.Name, val)
	return nil
}

func (vm *customNodeVM) opGetPath(op CustomNodeOp) error {
	from := vm.getVar(op.From)
	path := vm.resolvePathParts(op.Path)
	result := getNestedValue(from, path)
	if result == nil {
		result = cloneValue(op.Default)
	}
	if op.As != "" {
		vm.variables[op.As] = result
	}
	if op.To != "" {
		vm.setVar(op.To, result)
	}
	return nil
}

func (vm *customNodeVM) opSetPath(op CustomNodeOp) error {
	target := vm.getVar(op.Target)
	path := vm.resolvePathParts(op.Path)
	value := vm.resolveValue(op.Value)
	setNestedValue(target, path, value)
	return nil
}

func (vm *customNodeVM) opReturn(op CustomNodeOp) error {
	for name, valRef := range op.Outputs {
		vm.outputs[name] = vm.resolveValue(valRef)
	}
	return nil
}

func (vm *customNodeVM) opCallFunc(op CustomNodeOp) error {
	fn, ok := getCustomGoFunc(op.Func)
	if !ok {
		return fmt.Errorf("go function not registered: %s", op.Func)
	}
	// 解析 args（支持值引用）
	var args map[string]interface{}
	if op.Args != nil {
		args = make(map[string]interface{})
		if raw, ok := op.Args.(map[string]interface{}); ok {
			for k, v := range raw {
				args[k] = vm.resolveValue(v)
			}
		}
	}
	result, err := fn(vm.ctx, args)
	if err != nil {
		return err
	}
	if op.As != "" {
		vm.variables[op.As] = result
	}
	if op.To != "" {
		vm.setVar(op.To, result)
	}
	return nil
}

// ==================== 辅助方法 ====================

func (vm *customNodeVM) resolvePath(p string) string {
	// 支持 {var} 替换
	result := p
	for name, val := range vm.variables {
		result = strings.ReplaceAll(result, "{"+name+"}", fmt.Sprintf("%v", val))
	}
	return result
}

func (vm *customNodeVM) resolvePathParts(path []string) []string {
	result := make([]string, len(path))
	for i, p := range path {
		result[i] = vm.resolvePath(p)
	}
	return result
}

func (vm *customNodeVM) resolveValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		// 可能是 opValueRef
		if input, ok := val["input"].(string); ok && input != "" {
			if inp, exists := vm.inputs[input]; exists && !isEmptyValue(inp) {
				return inp
			}
		}
		if prop, ok := val["property"].(string); ok && prop != "" {
			if p, exists := vm.node.Properties[prop]; exists && !isEmptyValue(p) {
				return p
			}
		}
		if fb, ok := val["fallback"]; ok {
			return vm.resolveValue(fb)
		}
		if expr, ok := val["expr"].(string); ok && expr != "" {
			return vm.evalExpr(expr)
		}
		if get, ok := val["get"].(string); ok && get != "" {
			return vm.getVar(get)
		}
		// 普通 map，直接返回
		return val
	case string:
		// 检查是否是变量引用（以$开头或包含{var}）
		if strings.HasPrefix(val, "$") {
			return vm.getVar(val[1:])
		}
		// 简单的 {var} 替换
		if strings.Contains(val, "{") && strings.Contains(val, "}") {
			result := val
			for name, varVal := range vm.variables {
				result = strings.ReplaceAll(result, "{"+name+"}", fmt.Sprintf("%v", varVal))
			}
			return result
		}
		return val
	default:
		return v
	}
}

func (vm *customNodeVM) getVar(name string) interface{} {
	if v, ok := vm.variables[name]; ok {
		return v
	}
	// 支持嵌套路径，如 record.num
	parts := strings.Split(name, ".")
	if len(parts) > 1 {
		base := vm.getVar(parts[0])
		return getNestedValue(base, parts[1:])
	}
	return nil
}

func (vm *customNodeVM) setVar(name string, value interface{}) {
	// 支持嵌套设置，如 record.num
	parts := strings.Split(name, ".")
	if len(parts) == 1 {
		vm.variables[name] = value
		return
	}
	base := vm.getVar(parts[0])
	if base == nil {
		base = make(map[string]interface{})
		vm.variables[parts[0]] = base
	}
	setNestedValue(base, parts[1:], value)
}

// evalExpr 简单的表达式求值
func (vm *customNodeVM) evalExpr(expr string) interface{} {
	expr = strings.TrimSpace(expr)

	// 替换变量（支持点号路径）
	expr = vm.replaceExprVars(expr)

	// 尝试解析为数字比较
	if strings.Contains(expr, "==") {
		parts := strings.SplitN(expr, "==", 2)
		a := vm.evalSimple(strings.TrimSpace(parts[0]))
		b := vm.evalSimple(strings.TrimSpace(parts[1]))
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
	if strings.Contains(expr, "!=") {
		parts := strings.SplitN(expr, "!=", 2)
		a := vm.evalSimple(strings.TrimSpace(parts[0]))
		b := vm.evalSimple(strings.TrimSpace(parts[1]))
		return fmt.Sprintf("%v", a) != fmt.Sprintf("%v", b)
	}

	// 简单的算术
	return vm.evalSimple(expr)
}

// replaceExprVars 替换表达式中的变量引用（支持点号路径）
func (vm *customNodeVM) replaceExprVars(expr string) string {
	// 匹配标识符：字母数字下划线和点号
	re := regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_.]*`)
	return re.ReplaceAllStringFunc(expr, func(match string) string {
		// 跳过纯数字（虽然正则不会匹配到以数字开头的）
		if val := vm.getVar(match); val != nil {
			switch v := val.(type) {
			case float64, int, bool:
				return fmt.Sprintf("%v", v)
			case string:
				return v
			default:
				return fmt.Sprintf("%v", v)
			}
		}
		return match
	})
}

func (vm *customNodeVM) evalSimple(expr string) interface{} {
	expr = strings.TrimSpace(expr)

	// 处理 max(a, b)
	if strings.HasPrefix(expr, "max(") && strings.HasSuffix(expr, ")") {
		inner := expr[4 : len(expr)-1]
		parts := strings.Split(inner, ",")
		if len(parts) == 2 {
			a := toFloat(vm.evalSimple(strings.TrimSpace(parts[0])))
			b := toFloat(vm.evalSimple(strings.TrimSpace(parts[1])))
			if a > b {
				return a
			}
			return b
		}
	}

	// 加法 a + b
	if idx := strings.Index(expr, "+"); idx > 0 {
		a := toFloat(vm.evalSimple(strings.TrimSpace(expr[:idx])))
		b := toFloat(vm.evalSimple(strings.TrimSpace(expr[idx+1:])))
		return a + b
	}
	// 减法 a - b
	if idx := strings.Index(expr, "-"); idx > 0 {
		a := toFloat(vm.evalSimple(strings.TrimSpace(expr[:idx])))
		b := toFloat(vm.evalSimple(strings.TrimSpace(expr[idx+1:])))
		return a - b
	}

	// 数字
	if f, err := strconv.ParseFloat(expr, 64); err == nil {
		return f
	}
	// 布尔
	if expr == "true" {
		return true
	}
	if expr == "false" {
		return false
	}
	return expr
}

// ==================== 通用工具函数 ====================

// safeEvalMath 安全的数学表达式求值（仅支持 + - * / 和括号、数字）
func safeEvalMath(expr string) (float64, error) {
	expr = strings.TrimSpace(expr)
	// 仅允许合法字符
	for _, c := range expr {
		if !((c >= '0' && c <= '9') || c == '.' || c == '+' || c == '-' || c == '*' || c == '/' || c == '(' || c == ')' || c == ' ') {
			return 0, fmt.Errorf("invalid character in math expr: %c", c)
		}
	}
	result, err := evalMathExpr(expr)
	if err != nil {
		return 0, err
	}
	return result, nil
}

func evalMathExpr(expr string) (float64, error) {
	// 简单的递归下降解析器
	expr = strings.ReplaceAll(expr, " ", "")
	pos := 0
	var parseExpr func() (float64, error)
	var parseTerm func() (float64, error)
	var parseFactor func() (float64, error)

	parseExpr = func() (float64, error) {
		result, err := parseTerm()
		if err != nil {
			return 0, err
		}
		for pos < len(expr) && (expr[pos] == '+' || expr[pos] == '-') {
			op := expr[pos]
			pos++
			right, err := parseTerm()
			if err != nil {
				return 0, err
			}
			if op == '+' {
				result += right
			} else {
				result -= right
			}
		}
		return result, nil
	}

	parseTerm = func() (float64, error) {
		result, err := parseFactor()
		if err != nil {
			return 0, err
		}
		for pos < len(expr) && (expr[pos] == '*' || expr[pos] == '/') {
			op := expr[pos]
			pos++
			right, err := parseFactor()
			if err != nil {
				return 0, err
			}
			if op == '*' {
				result *= right
			} else {
				if right == 0 {
					return 0, fmt.Errorf("division by zero")
				}
				result /= right
			}
		}
		return result, nil
	}

	parseFactor = func() (float64, error) {
		if pos >= len(expr) {
			return 0, fmt.Errorf("unexpected end of expression")
		}
		if expr[pos] == '(' {
			pos++
			result, err := parseExpr()
			if err != nil {
				return 0, err
			}
			if pos >= len(expr) || expr[pos] != ')' {
				return 0, fmt.Errorf("missing closing parenthesis")
			}
			pos++
			return result, nil
		}
		if expr[pos] == '-' {
			pos++
			val, err := parseFactor()
			if err != nil {
				return 0, err
			}
			return -val, nil
		}
		// 解析数字
		start := pos
		for pos < len(expr) && ((expr[pos] >= '0' && expr[pos] <= '9') || expr[pos] == '.') {
			pos++
		}
		if start == pos {
			return 0, fmt.Errorf("expected number at pos %d", pos)
		}
		return strconv.ParseFloat(expr[start:pos], 64)
	}

	return parseExpr()
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	}
	return 0
}

func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case float64:
		return val == 0
	case int:
		return val == 0
	}
	return false
}

func cloneValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var result interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

func getNestedValue(obj interface{}, path []string) interface{} {
	current := obj
	for _, key := range path {
		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[key]
			if !ok {
				return nil
			}
		default:
			return nil
		}
	}
	return current
}

func setNestedValue(obj interface{}, path []string, value interface{}) {
	if len(path) == 0 {
		return
	}
	current := obj
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		switch v := current.(type) {
		case map[string]interface{}:
			next, ok := v[key]
			if !ok {
				next = make(map[string]interface{})
				v[key] = next
			}
			current = next
		default:
			return
		}
	}
	lastKey := path[len(path)-1]
	switch v := current.(type) {
	case map[string]interface{}:
		v[lastKey] = value
	}
}
