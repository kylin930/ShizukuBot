package plugin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
	"github.com/tencent-connect/botgo/dto"
)

var SaveTempImage func(data []byte) string

var (
	atBotRE   = regexp.MustCompile(`<@!?[0-9A-Fa-f]+>`)
	atUserRE  = regexp.MustCompile(`<@[0-9A-Fa-f]+>`)
	spaceChar = " \u00A0"
)

func cleanMessage(input string) string {
	cleaned := atBotRE.ReplaceAllString(input, "")
	cleaned = strings.Trim(cleaned, spaceChar)
	return cleaned
}

func cleanMessageWithContext(ctx *EventContext, input string) string {
	botIDs := getBotMentionIDs(ctx)
	cleaned := input
	if len(botIDs) > 0 {
		for _, bid := range botIDs {
			re := regexp.MustCompile(`<@!?` + regexp.QuoteMeta(bid) + `>`)
			cleaned = re.ReplaceAllString(cleaned, "")
		}
	} else {
		cleaned = atBotRE.ReplaceAllString(cleaned, "")
	}
	cleaned = strings.Trim(cleaned, spaceChar)
	return cleaned
}

func getBotMentionIDs(ctx *EventContext) []string {
	var ids []string
	var mentions []*dto.User
	switch v := ctx.EventData.(type) {
	case *dto.WSGroupATMessageData:
		mentions = v.Mentions
	case *dto.WSGroupMessageData:
		mentions = v.Mentions
	case *dto.WSC2CMessageData:
		mentions = v.Mentions
	case *dto.WSATMessageData:
		mentions = v.Mentions
	case *dto.WSDirectMessageData:
		mentions = v.Mentions
	}
	if mentions != nil {
		for _, m := range mentions {
			if m != nil && m.IsYou {
				ids = append(ids, m.ID)
			}
		}
	}
	return ids
}

func extractMentionIDs(ctx *EventContext) ([]string, []string) {
	var ids []string
	var names []string
	var mentions []*dto.User
	switch v := ctx.EventData.(type) {
	case *dto.WSGroupATMessageData:
		mentions = v.Mentions
	case *dto.WSGroupMessageData:
		mentions = v.Mentions
	case *dto.WSC2CMessageData:
		mentions = v.Mentions
	case *dto.WSATMessageData:
		mentions = v.Mentions
	case *dto.WSDirectMessageData:
		mentions = v.Mentions
	}
	if mentions != nil {
		for _, m := range mentions {
			if m == nil || m.Bot || m.IsYou {
				continue
			}
			ids = append(ids, m.ID)
			names = append(names, m.Username)
		}
	}
	if len(ids) == 0 {
		botIDs := getBotMentionIDs(ctx)
		botIDSet := make(map[string]bool)
		for _, bid := range botIDs {
			botIDSet[bid] = true
		}
		re := regexp.MustCompile(`<@!?([0-9A-Fa-f]+)>`)
		matches := re.FindAllStringSubmatch(ctx.Message, -1)
		for _, m := range matches {
			if len(m) > 1 {
				if botIDSet[m[1]] {
					continue
				}
				ids = append(ids, m[1])
				names = append(names, "")
			}
		}
	}
	return ids, names
}

func extractAttachments(ctx *EventContext) []*dto.MessageAttachment {
	switch v := ctx.EventData.(type) {
	case *dto.WSGroupATMessageData:
		return v.Attachments
	case *dto.WSC2CMessageData:
		return v.Attachments
	case *dto.WSATMessageData:
		return v.Attachments
	case *dto.WSDirectMessageData:
		return v.Attachments
	}
	return nil
}

type EventContext struct {
	BotID             string
	EventType         string
	EventData         interface{}
	Variables         map[string]interface{}
	Message           string
	UserID            string
	GroupID           string
	ChannelID         string
	GuildID           string
	MessageID         string
	PluginIDs         []string
	ExecutedPlugins   map[string]bool // 防重入：标记已执行的插件
	ReplyToUser       func(content string) error
	ReplyToGroup      func(content string) error
	ReplyImageToUser  func(imageURL string) error
	ReplyImageToGroup func(imageURL string) error
	ReplyRichMediaToUser  func(text, imageURL string) error
	ReplyRichMediaToGroup func(text, imageURL string) error
	ReplyMarkdownToUser   func(content string) error
	ReplyMarkdownToGroup  func(content string) error
	ReplyKeyboardToUser   func(markdown string, keyboardJSON string) error
	ReplyKeyboardToGroup  func(markdown string, keyboardJSON string) error
	ReplyVideoToUser      func(videoURL string) error
	ReplyVideoToGroup     func(videoURL string) error
	LogDebug          func(botID, message string)
}

type GraphData struct {
	Nodes []NodeData `json:"nodes"`
	Links []LinkData `json:"links"`
}

type SizeData [2]float64

func (s *SizeData) UnmarshalJSON(data []byte) error {
	var arr [2]float64
	if err := json.Unmarshal(data, &arr); err == nil {
		*s = arr
		return nil
	}

	var obj struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		*s = [2]float64{obj.Width, obj.Height}
		return nil
	}

	return fmt.Errorf("invalid size format")
}

type NodeData struct {
	ID         int                    `json:"id"`
	Type       string                 `json:"type"`
	Title      string                 `json:"title"`
	Pos        [2]float64             `json:"pos"`
	Size       SizeData               `json:"size"`
	Mode       int                    `json:"mode"`
	Flags      map[string]interface{} `json:"flags"`
	Order      int                    `json:"order"`
	Inputs     []SlotData             `json:"inputs"`
	Outputs    []SlotData             `json:"outputs"`
	Properties map[string]interface{} `json:"properties"`
}

type SlotType string

func (s *SlotType) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = SlotType(str)
		return nil
	}

	var num int
	if err := json.Unmarshal(data, &num); err == nil {
		*s = SlotType(strconv.Itoa(num))
		return nil
	}

	return fmt.Errorf("invalid slot type format")
}

type SlotData struct {
	Name  string   `json:"name"`
	Type  SlotType `json:"type"`
	Link  int      `json:"link,omitempty"`
	Links []int    `json:"links,omitempty"`
}

type LinkData struct {
	ID          int    `json:"id"`
	Origin      int    `json:"origin_id"`
	OriginSlot  int    `json:"origin_slot"`
	Target      int    `json:"target_id"`
	TargetSlot  int    `json:"target_slot"`
	Type        string `json:"type"`
}

type ExecutionEngine struct {
	store             *PluginStore
	executors         map[string]NodeExecutor
	mu                sync.RWMutex
	graphCache        sync.Map // map[string]*parsedGraph，key=pluginID:updatedAt
	processedMessages sync.Map // map[string]time.Time，key=去重指纹，value=处理时间（定时清理）
}

// parsedGraph 是解析后的流程图缓存（只读，可并发共享）
type parsedGraph struct {
	nodes   map[int]*NodeData
	linkMap map[int]*LinkData
}

type NodeExecutor interface {
	Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error)
}

// GlobalCustomNodeStore 全局自定义节点存储（供 API 使用）
var GlobalCustomNodeStore *CustomNodeStore

// RegisterPluginCustomNodes 注册插件内嵌的自定义节点
func (e *ExecutionEngine) RegisterPluginCustomNodes(plugin *Plugin) int {
	if plugin.CustomNodes == nil || len(plugin.CustomNodes) == 0 {
		return 0
	}
	var defs []*CustomNodeDef
	if err := json.Unmarshal(plugin.CustomNodes, &defs); err != nil {
		fmt.Printf("[CustomNode] failed to parse plugin %s custom_nodes: %v\n", plugin.ID, err)
		return 0
	}
	count := 0
	for _, def := range defs {
		if def == nil || def.Type == "" {
			continue
		}
		e.RegisterExecutor(def.Type, &CustomNodeExecutor{def: def})
		count++
	}
	return count
}

// ParsePluginCustomNodes 解析插件内嵌的自定义节点定义（供前端使用）
func ParsePluginCustomNodes(plugin *Plugin) []*CustomNodeDef {
	if plugin.CustomNodes == nil || len(plugin.CustomNodes) == 0 {
		return nil
	}
	var defs []*CustomNodeDef
	if err := json.Unmarshal(plugin.CustomNodes, &defs); err != nil {
		return nil
	}
	return defs
}

func NewExecutionEngine(store *PluginStore) *ExecutionEngine {
	engine := &ExecutionEngine{
		store:     store,
		executors: make(map[string]NodeExecutor),
	}
	engine.registerBuiltinExecutors()

	// 加载自定义节点
	cnStore, err := NewCustomNodeStore("data/custom_nodes")
	if err != nil {
		fmt.Printf("[CustomNode] failed to init: %v\n", err)
	} else {
		GlobalCustomNodeStore = cnStore
		cnStore.RegisterAll(engine)
		fmt.Printf("[CustomNode] loaded %d custom nodes\n", len(cnStore.nodes))
	}

	// 启动去重记录定时清理（每 1 分钟清理一次，保留 5 分钟窗口）
	go engine.cleanupProcessedMessages()

	return engine
}

// buildDedupKey 生成消息去重指纹。用（来源ID + 发送者ID + 消息内容）组合，
// 解决同一条消息被 QQ 以 GROUP_AT_MESSAGE_CREATE 和 GROUP_MESSAGE_CREATE 两个事件重复推送的问题。
func (e *ExecutionEngine) buildDedupKey(ctx *EventContext) string {
	// 至少要有来源（群/频道）和发送者，否则不去重
	sourceID := ctx.GroupID
	if sourceID == "" {
		sourceID = ctx.ChannelID
	}
	if sourceID == "" || ctx.UserID == "" {
		return ""
	}
	// 用"|"分隔，避免字段值互相渗透
	return sourceID + "|" + ctx.UserID + "|" + ctx.Message
}

// cleanupProcessedMessages 定期清理过期的去重记录，避免内存无限增长。
// 保留窗口 5 分钟，足够覆盖 QQ 双事件推送的时间差（通常在毫秒级）。
func (e *ExecutionEngine) cleanupProcessedMessages() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-5 * time.Minute)
		e.processedMessages.Range(func(key, value interface{}) bool {
			if t, ok := value.(time.Time); ok && t.Before(cutoff) {
				e.processedMessages.Delete(key)
			}
			return true
		})
	}
}

func (e *ExecutionEngine) registerBuiltinExecutors() {
	// 事件节点
	e.RegisterExecutor("event/on_group_message", &OnGroupMessageExecutor{})
	e.RegisterExecutor("event/on_c2c_message", &OnC2CMessageExecutor{})
	e.RegisterExecutor("event/on_guild_message", &OnGuildMessageExecutor{})
	e.RegisterExecutor("event/on_direct_message", &OnDirectMessageExecutor{})
	e.RegisterExecutor("event/on_command", &OnCommandExecutor{})
	e.RegisterExecutor("event/on_c2c_friend", &OnC2CFriendExecutor{})
	e.RegisterExecutor("event/on_interaction", &OnInteractionExecutor{})
	e.RegisterExecutor("event/on_subscribe_msg_status", &OnSubscribeMsgStatusExecutor{})
	e.RegisterExecutor("event/on_group_manage", &OnGroupManageExecutor{})
	e.RegisterExecutor("event/on_c2c_manage", &OnC2CManageExecutor{})

	// 发送消息节点
	e.RegisterExecutor("action/send_message", &SendMessageExecutor{})
	e.RegisterExecutor("action/send_group_message", &SendGroupMessageExecutor{})
	e.RegisterExecutor("action/send_c2c_message", &SendC2CMessageExecutor{})
	e.RegisterExecutor("action/send_image", &SendImageExecutor{})
	e.RegisterExecutor("action/send_rich_media", &SendRichMediaExecutor{})
	e.RegisterExecutor("action/send_markdown", &SendMarkdownExecutor{})
	e.RegisterExecutor("action/send_buttons", &SendButtonsExecutor{})
	e.RegisterExecutor("action/send_markdown_with_buttons", &SendMarkdownWithButtonsExecutor{})
	e.RegisterExecutor("action/send_video", &SendVideoExecutor{})
	e.RegisterExecutor("action/recall_message", &RecallMessageExecutor{})

	// 消息处理节点
	e.RegisterExecutor("message/extract_mention_user", &ExtractMentionUserExecutor{})
	e.RegisterExecutor("message/parse_images", &ParseImagesExecutor{})
	e.RegisterExecutor("message/parse_command", &ParseCommandExecutor{})

	// HTML 渲染节点
	e.RegisterExecutor("render/html_to_image", &HTMLToImageExecutor{})
	e.RegisterExecutor("render/html_template", &HTMLTemplateExecutor{})

	// 文件操作节点
	e.RegisterExecutor("file/read", &FileReadExecutor{})
	e.RegisterExecutor("file/write", &FileWriteExecutor{})

	// 条件判断节点
	e.RegisterExecutor("condition/text_contains", &TextContainsExecutor{})
	e.RegisterExecutor("condition/text_equals", &TextEqualsExecutor{})
	e.RegisterExecutor("condition/text_matches", &TextMatchesExecutor{})
	e.RegisterExecutor("condition/number_compare", &NumberCompareExecutor{})
	e.RegisterExecutor("condition/is_empty", &IsEmptyExecutor{})

	// 逻辑节点
	e.RegisterExecutor("logic/branch", &BranchExecutor{})
	e.RegisterExecutor("logic/and", &AndExecutor{})
	e.RegisterExecutor("logic/or", &OrExecutor{})
	e.RegisterExecutor("logic/not", &NotExecutor{})

	// 文本处理节点
	e.RegisterExecutor("text/template", &TemplateExecutor{})
	e.RegisterExecutor("text/concat", &ConcatExecutor{})
	e.RegisterExecutor("text/substring", &SubstringExecutor{})
	e.RegisterExecutor("text/replace", &ReplaceExecutor{})
	e.RegisterExecutor("text/split", &SplitExecutor{})
	e.RegisterExecutor("text/length", &TextLengthExecutor{})
	e.RegisterExecutor("text/to_upper", &ToUpperExecutor{})
	e.RegisterExecutor("text/to_lower", &ToLowerExecutor{})
	e.RegisterExecutor("text/trim", &TrimExecutor{})
	e.RegisterExecutor("text/find", &TextFindExecutor{})
	e.RegisterExecutor("text/char_at", &TextCharAtExecutor{})
	e.RegisterExecutor("text/regex_replace", &TextRegexReplaceExecutor{})

	// 数学运算节点
	e.RegisterExecutor("math/number", &MathNumberExecutor{})
	e.RegisterExecutor("math/add", &MathAddExecutor{})
	e.RegisterExecutor("math/subtract", &MathSubtractExecutor{})
	e.RegisterExecutor("math/multiply", &MathMultiplyExecutor{})
	e.RegisterExecutor("math/divide", &MathDivideExecutor{})
	e.RegisterExecutor("math/modulo", &MathModuloExecutor{})
	e.RegisterExecutor("math/random", &MathRandomExecutor{})
	e.RegisterExecutor("math/round", &MathRoundExecutor{})
	e.RegisterExecutor("math/floor", &MathFloorExecutor{})
	e.RegisterExecutor("math/ceil", &MathCeilExecutor{})
	e.RegisterExecutor("math/abs", &MathAbsExecutor{})
	e.RegisterExecutor("math/type_convert", &TypeConvertExecutor{})
	e.RegisterExecutor("math/to_number", &ToNumberExecutor{})
	e.RegisterExecutor("math/sqrt", &MathSqrtExecutor{})
	e.RegisterExecutor("math/sin", &MathSinExecutor{})
	e.RegisterExecutor("math/pi", &MathPiExecutor{})
	e.RegisterExecutor("math/is_even", &MathIsEvenExecutor{})
	e.RegisterExecutor("math/sum", &MathSumExecutor{})
	e.RegisterExecutor("math/clamp", &MathClampExecutor{})
	e.RegisterExecutor("math/random_float", &MathRandomFloatExecutor{})

	// 网络请求节点
	e.RegisterExecutor("http/request", &HTTPRequestExecutor{})
	e.RegisterExecutor("json/parse", &JSONParseExecutor{})
	e.RegisterExecutor("json/generate", &JSONGenerateExecutor{})
	e.RegisterExecutor("http/url_encode", &URLEncodeExecutor{})
	e.RegisterExecutor("http/url_decode", &URLDecodeExecutor{})

	// 变量节点
	e.RegisterExecutor("variable/set", &SetVariableExecutor{})
	e.RegisterExecutor("variable/get", &GetVariableExecutor{})
	e.RegisterExecutor("variable/delete", &DeleteVariableExecutor{})
	e.RegisterExecutor("variable/constant", &ConstantExecutor{})

	// 时间节点
	e.RegisterExecutor("time/now", &TimeNowExecutor{})
	e.RegisterExecutor("time/format", &TimeFormatExecutor{})
	e.RegisterExecutor("time/calculate", &TimeCalculateExecutor{})
	e.RegisterExecutor("time/delay", &DelayExecutor{})

	// 指令节点
	e.RegisterExecutor("command/create", &CreateCommandExecutor{})
	e.RegisterExecutor("command/permission", &PermissionCheckExecutor{})

	// 上下文信息节点
	e.RegisterExecutor("context/get_user_id", &GetUserIDExecutor{})
	e.RegisterExecutor("context/get_group_id", &GetGroupIDExecutor{})
	e.RegisterExecutor("context/get_channel_id", &GetChannelIDExecutor{})
	e.RegisterExecutor("context/get_guild_id", &GetGuildIDExecutor{})
	e.RegisterExecutor("context/get_message_id", &GetMessageIDExecutor{})
	e.RegisterExecutor("context/get_source", &GetSourceExecutor{})
	e.RegisterExecutor("context/get_plugin_id", &GetPluginIDExecutor{})

	// 日志节点
	e.RegisterExecutor("log/debug", &LogDebugExecutor{})

	// 时间节点
	e.RegisterExecutor("time/timestamp", &TimestampExecutor{})
	e.RegisterExecutor("time/format_custom", &TimeFormatCustomExecutor{})

	// 代码节点
	e.RegisterExecutor("code/go", &CodeGoExecutor{})

	// 列表节点
	e.RegisterExecutor("list/create_empty", &ListCreateEmptyExecutor{})
	e.RegisterExecutor("list/create", &ListCreateExecutor{})
	e.RegisterExecutor("list/add_repeat", &ListAddRepeatExecutor{})
	e.RegisterExecutor("list/length", &ListLengthExecutor{})
	e.RegisterExecutor("list/is_empty", &ListIsEmptyExecutor{})
	e.RegisterExecutor("list/find", &ListFindExecutor{})
	e.RegisterExecutor("list/get", &ListGetExecutor{})
	e.RegisterExecutor("list/set", &ListSetExecutor{})
	e.RegisterExecutor("list/sublist", &ListSubListExecutor{})
	e.RegisterExecutor("list/from_text", &ListFromTextExecutor{})
	e.RegisterExecutor("list/sort", &ListSortExecutor{})
	e.RegisterExecutor("list/delete", &ListDeleteExecutor{})
}

func (e *ExecutionEngine) RegisterExecutor(nodeType string, executor NodeExecutor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executors[nodeType] = executor
}

func (e *ExecutionEngine) ProcessEvent(ctx *EventContext) bool {
	// 消息级去重：同一条消息可能同时触发 GROUP_AT_MESSAGE_CREATE + GROUP_MESSAGE_CREATE 两个事件
	// 用（群ID/频道ID + 发送者ID + 消息内容）生成指纹，比单纯 MessageID 更可靠
	if dedupKey := e.buildDedupKey(ctx); dedupKey != "" {
		now := time.Now()
		if _, already := e.processedMessages.LoadOrStore(dedupKey, now); already {
			return false
		}
	}
	if ctx.ExecutedPlugins == nil {
		ctx.ExecutedPlugins = make(map[string]bool)
	}
	// 前置检查：是否有任何插件的 event 节点命中？命中才执行并计入统计
	plugins := e.getPluginsForExecution(ctx)
	var hitPlugins []*Plugin
	for _, p := range plugins {
		if e.hasEventNodeHit(p, ctx) {
			hitPlugins = append(hitPlugins, p)
		}
	}
	if len(hitPlugins) == 0 {
		return false // 无命中，跳过执行，不计统计
	}

	// 只执行命中 event 的插件
	for _, p := range hitPlugins {
		if err := e.ExecutePlugin(ctx, p); err != nil {
			errMsg := fmt.Sprintf("⚠️ 插件执行出错：%v", err)
			fmt.Printf("[Plugin] Error executing plugin %s: %v\n", p.ID, err)
			// 将错误信息回复给用户
			if ctx.ReplyToGroup != nil && ctx.GroupID != "" {
				_ = ctx.ReplyToGroup(errMsg)
			} else if ctx.ReplyToUser != nil && ctx.UserID != "" {
				_ = ctx.ReplyToUser(errMsg)
			}
		}
	}
	return true
}

// hasEventNodeHit 检查该插件是否有任何 event 节点命中当前事件
func (e *ExecutionEngine) hasEventNodeHit(plugin *Plugin, ctx *EventContext) bool {
	pg := e.getParsedGraph(plugin)
	nodes := pg.nodes
	for _, node := range nodes {
		if isEventNode(node.Type) && shouldExecute(node, ctx) {
			return true
		}
	}
	return false
}

func (e *ExecutionEngine) ProcessEventAndGetReplies(ctx *EventContext) ([]string, error) {
	return e.ProcessEventAndGetRepliesWithPlugin(ctx, "")
}

// ProcessEventAndGetRepliesWithPlugin 执行插件并捕获回复
// 如果 pluginID 不为空，则只执行指定的插件（无论是否命中事件，便于沙盒测试）；否则执行所有命中事件的已启用插件
func (e *ExecutionEngine) ProcessEventAndGetRepliesWithPlugin(ctx *EventContext, pluginID string) ([]string, error) {
	if ctx.ExecutedPlugins == nil {
		ctx.ExecutedPlugins = make(map[string]bool)
	}
	var plugins []*Plugin
	if pluginID != "" {
		// 只执行指定插件（无论是否启用，便于测试）
		if p, ok := e.store.Get(pluginID); ok {
			plugins = []*Plugin{p}
		}
	} else {
		// 只执行命中事件的插件
		allPlugins := e.getPluginsForExecution(ctx)
		for _, p := range allPlugins {
			if e.hasEventNodeHit(p, ctx) {
				plugins = append(plugins, p)
			}
		}
	}
	var replies []string
	captureReply := func(content string) error {
		replies = append(replies, content)
		return nil
	}
	captureImageReply := func(imageURL string) error {
		replies = append(replies, fmt.Sprintf("[图片] %s", imageURL))
		return nil
	}
	captureRichMediaReply := func(text, imageURL string) error {
		replies = append(replies, fmt.Sprintf("%s\n[图片] %s", text, imageURL))
		return nil
	}
	origReplyToUser := ctx.ReplyToUser
	origReplyToGroup := ctx.ReplyToGroup
	origReplyImageToUser := ctx.ReplyImageToUser
	origReplyImageToGroup := ctx.ReplyImageToGroup
	origReplyRichMediaToUser := ctx.ReplyRichMediaToUser
	origReplyRichMediaToGroup := ctx.ReplyRichMediaToGroup
	ctx.ReplyToUser = captureReply
	ctx.ReplyToGroup = captureReply
	ctx.ReplyImageToUser = captureImageReply
	ctx.ReplyImageToGroup = captureImageReply
	ctx.ReplyRichMediaToUser = captureRichMediaReply
	ctx.ReplyRichMediaToGroup = captureRichMediaReply

	for _, p := range plugins {
		if err := e.ExecutePlugin(ctx, p); err != nil {
			errMsg := fmt.Sprintf("⚠️ 插件执行出错：%v", err)
			// 沙盒模式下也将错误信息捕获为回复
			replies = append(replies, errMsg)
		}
	}

	ctx.ReplyToUser = origReplyToUser
	ctx.ReplyToGroup = origReplyToGroup
	ctx.ReplyImageToUser = origReplyImageToUser
	ctx.ReplyImageToGroup = origReplyImageToGroup
	ctx.ReplyRichMediaToUser = origReplyRichMediaToUser
	ctx.ReplyRichMediaToGroup = origReplyRichMediaToGroup
	return replies, nil
}

// getPluginsForExecution 根据ctx.PluginIDs决定执行的插件列表
// 如果 ctx.PluginIDs 不为空，则只执行这些ID对应的已启用插件
// 否则执行所有启用的插件（兼容旧逻辑）
func (e *ExecutionEngine) getPluginsForExecution(ctx *EventContext) []*Plugin {
	if len(ctx.PluginIDs) > 0 {
		return e.store.GetEnabledPluginsByIDs(ctx.PluginIDs)
	}
	return e.store.GetEnabledPlugins(ctx.BotID)
}

func (e *ExecutionEngine) ExecutePlugin(ctx *EventContext, plugin *Plugin) error {
	// 防重入：同一个插件对同一条消息只执行一次
	if ctx.ExecutedPlugins != nil {
		if ctx.ExecutedPlugins[plugin.ID] {
			return nil
		}
		ctx.ExecutedPlugins[plugin.ID] = true
	}

	// 注册该插件内嵌的自定义节点
	if count := e.RegisterPluginCustomNodes(plugin); count > 0 {
		// 清空缓存，确保新注册的节点能被识别
		e.graphCache.Delete(fmt.Sprintf("%s:%d", plugin.ID, plugin.UpdatedAt))
	}

	pg := e.getParsedGraph(plugin)
	nodes := pg.nodes
	linkMap := pg.linkMap

	outputData := make(map[int]map[string]interface{})
	executed := make(map[int]bool)

	var executeNode func(nodeID int) error
	executeNode = func(nodeID int) error {
		if executed[nodeID] {
			return nil
		}
		executed[nodeID] = true

		node, ok := nodes[nodeID]
		if !ok {
			return fmt.Errorf("node %d not found", nodeID)
		}

		inputs := make(map[string]interface{})
		for i, input := range node.Inputs {
			if input.Link < 0 {
				continue
			}
			link, ok := linkMap[input.Link]
			if !ok {
				continue
			}
			sourceNodeID := link.Origin
			if err := executeNode(sourceNodeID); err != nil {
				return err
			}
			out, ok := outputData[sourceNodeID]
			if !ok || len(out) == 0 {
				continue
			}
			// 触发类型连接(type=-1)不传递数据，只用于控制流
			if string(input.Type) == "-1" {
				continue
			}
			inputSlotName := node.Inputs[i].Name
			// 用link.OriginSlot找到源节点的输出槽名，尝试匹配
			sourceNode, sourceExists := nodes[sourceNodeID]
			if sourceExists && link.OriginSlot < len(sourceNode.Outputs) {
				sourceSlotName := sourceNode.Outputs[link.OriginSlot].Name
				if v, ok := out[sourceSlotName]; ok {
					inputs[inputSlotName] = v
					continue
				}
			}
			// 槽名不匹配时（executor返回英文key，槽名是中文），取第一个值
			for _, v := range out {
				inputs[inputSlotName] = v
				break
			}
		}

		executor, ok := e.executors[node.Type]
		if !ok {
			return fmt.Errorf("no executor for node type: %s", node.Type)
		}

		output, err := executor.Execute(ctx, node, inputs)
		if err != nil {
			return err
		}
		outputData[nodeID] = output

		if node.Outputs != nil {
			for _, outPort := range node.Outputs {
				// 只有当该输出端口有真值时才触发下游，
				// 避免条件节点的"成立/不成立"两个分支同时被执行
				shouldTrigger := true
				if output != nil {
					if val, exists := output[outPort.Name]; exists {
						shouldTrigger = isTruthy(val)
					}
				}
				if !shouldTrigger {
					continue
				}
				if outPort.Links != nil {
					for _, linkID := range outPort.Links {
						if link, ok := linkMap[linkID]; ok {
							if err := executeNode(link.Target); err != nil {
								return err
							}
						}
					}
				}
			}
		}

		return nil
	}

	for _, node := range nodes {
		if isEventNode(node.Type) {
			if shouldExecute(node, ctx) {
				if err := executeNode(node.ID); err != nil {
					return err
				}
				break // 一次事件只触发第一个命中的事件节点，避免重复执行
			}
		}
	}

	return nil
}

// getParsedGraph 获取插件流程图的解析结果（带缓存）
// 缓存 key 包含 UpdatedAt，插件更新后自动失效
func (e *ExecutionEngine) getParsedGraph(plugin *Plugin) *parsedGraph {
	cacheKey := plugin.ID + ":" + strconv.FormatInt(plugin.UpdatedAt, 10)
	if v, ok := e.graphCache.Load(cacheKey); ok {
		return v.(*parsedGraph)
	}

	pg := parseGraph(plugin.Graph)
	e.graphCache.Store(cacheKey, pg)
	return pg
}

// GetCommandList 扫描已启用插件图，收集所有 event/on_command 节点定义的指令。
// pluginIDs 非空时只扫描指定插件，否则扫描该 botID 下所有已启用插件。
// 返回格式 "prefix+command" 的列表，如 ["/help", "/ping"]；指令名为空的节点跳过。
func (e *ExecutionEngine) GetCommandList(pluginIDs []string, botID string) []string {
	var plugins []*Plugin
	if len(pluginIDs) > 0 {
		plugins = e.store.GetEnabledPluginsByIDs(pluginIDs)
	} else {
		plugins = e.store.GetEnabledPlugins(botID)
	}

	seen := make(map[string]bool)
	var commands []string
	for _, p := range plugins {
		pg := e.getParsedGraph(p)
		if pg == nil || pg.nodes == nil {
			continue
		}
		for _, node := range pg.nodes {
			if node.Type != "event/on_command" {
				continue
			}
			cmdName := ""
			if c, ok := node.Properties["command"].(string); ok {
				cmdName = c
			}
			if cmdName == "" {
				continue // 未填指令名的节点跳过
			}
			prefix := "/"
			if p, ok := node.Properties["prefix"].(string); ok && p != "" {
				prefix = p
			}
			full := prefix + cmdName
			if !seen[full] {
				seen[full] = true
				commands = append(commands, full)
			}
		}
	}
	return commands
}

// parseGraph 解析插件流程图 JSON
func parseGraph(graphData json.RawMessage) *parsedGraph {
	var rawGraph map[string]interface{}
	if err := json.Unmarshal(graphData, &rawGraph); err != nil {
		return &parsedGraph{
			nodes:   make(map[int]*NodeData),
			linkMap: make(map[int]*LinkData),
		}
	}

	nodes := make(map[int]*NodeData)
	if nodesRaw, ok := rawGraph["nodes"].([]interface{}); ok {
		for _, nodeRaw := range nodesRaw {
			nodeData, ok := nodeRaw.(map[string]interface{})
			if !ok {
				continue
			}
			nodeID := int(getFloatVal(nodeData, "id"))
			node := &NodeData{
				ID:    nodeID,
				Type:  getStrVal(nodeData, "type"),
				Title: getStrVal(nodeData, "title"),
			}

			if posRaw, ok := nodeData["pos"].([]interface{}); ok && len(posRaw) >= 2 {
				node.Pos = [2]float64{getFloatVal(posRaw[0], ""), getFloatVal(posRaw[1], "")}
			}
			if sizeRaw, ok := nodeData["size"].([]interface{}); ok && len(sizeRaw) >= 2 {
				node.Size = SizeData{getFloatVal(sizeRaw[0], ""), getFloatVal(sizeRaw[1], "")}
			} else if sizeRaw, ok := nodeData["size"].(map[string]interface{}); ok {
				// LiteGraph.js序列化的size格式是 {"0": width, "1": height}
				w := getFloatVal(sizeRaw, "0")
				h := getFloatVal(sizeRaw, "1")
				if w == 0 {
					w = getFloatVal(sizeRaw, "width")
				}
				if h == 0 {
					h = getFloatVal(sizeRaw, "height")
				}
				node.Size = SizeData{w, h}
			}

			node.Mode = int(getFloatVal(nodeData, "mode"))
			node.Order = int(getFloatVal(nodeData, "order"))

			if propsRaw, ok := nodeData["properties"].(map[string]interface{}); ok {
				node.Properties = propsRaw
			} else {
				node.Properties = make(map[string]interface{})
			}

			if flagsRaw, ok := nodeData["flags"].(map[string]interface{}); ok {
				node.Flags = flagsRaw
			} else {
				node.Flags = make(map[string]interface{})
			}

			if inputsRaw, ok := nodeData["inputs"].([]interface{}); ok {
				for _, inputRaw := range inputsRaw {
					if inputData, ok := inputRaw.(map[string]interface{}); ok {
						link := -1
						if linkVal, exists := inputData["link"]; exists && linkVal != nil {
							link = int(getFloatVal(linkVal, ""))
						}
						node.Inputs = append(node.Inputs, SlotData{
							Name:  getStrVal(inputData, "name"),
							Type:  SlotType(getStrVal(inputData, "type")),
							Link:  link,
						})
					}
				}
			}

			if outputsRaw, ok := nodeData["outputs"].([]interface{}); ok {
				for _, outputRaw := range outputsRaw {
					if outputData, ok := outputRaw.(map[string]interface{}); ok {
						slot := SlotData{
							Name:  getStrVal(outputData, "name"),
							Type:  SlotType(getStrVal(outputData, "type")),
							Link:  -1,
						}
						if linkVal, exists := outputData["link"]; exists && linkVal != nil {
							slot.Link = int(getFloatVal(linkVal, ""))
						}
						if linksRaw, ok := outputData["links"].([]interface{}); ok {
							for _, linkRaw := range linksRaw {
								slot.Links = append(slot.Links, int(getFloatVal(linkRaw, "")))
							}
						}
						if len(slot.Links) == 0 && slot.Link >= 0 {
							slot.Links = []int{slot.Link}
						}
						node.Outputs = append(node.Outputs, slot)
					}
				}
			}

			nodes[nodeID] = node
		}
	}

	linkMap := make(map[int]*LinkData)
	if linksRaw, ok := rawGraph["links"].([]interface{}); ok {
		for _, linkRaw := range linksRaw {
			if linkArr, ok := linkRaw.([]interface{}); ok && len(linkArr) >= 6 {
				linkID := int(getFloatVal(linkArr[0], ""))
				link := &LinkData{
					ID:         linkID,
					Origin:     int(getFloatVal(linkArr[1], "")),
					OriginSlot: int(getFloatVal(linkArr[2], "")),
					Target:     int(getFloatVal(linkArr[3], "")),
					TargetSlot: int(getFloatVal(linkArr[4], "")),
					Type:       getStrVal(linkArr[5], ""),
				}
				linkMap[linkID] = link
			} else if linkData, ok := linkRaw.(map[string]interface{}); ok {
				linkID := int(getFloatVal(linkData, "id"))
				link := &LinkData{
					ID:         linkID,
					Origin:     int(getFloatVal(linkData, "origin_id")),
					OriginSlot: int(getFloatVal(linkData, "origin_slot")),
					Target:     int(getFloatVal(linkData, "target_id")),
					TargetSlot: int(getFloatVal(linkData, "target_slot")),
					Type:       getStrVal(linkData, "type"),
				}
				linkMap[linkID] = link
			}
		}
	}

	return &parsedGraph{nodes: nodes, linkMap: linkMap}
}

func isEventNode(nodeType string) bool {
	return nodeType == "event/on_group_message" ||
		nodeType == "event/on_c2c_message" ||
		nodeType == "event/on_guild_message" ||
		nodeType == "event/on_direct_message" ||
		nodeType == "event/on_command" ||
		nodeType == "event/on_c2c_friend" ||
		nodeType == "event/on_interaction" ||
		nodeType == "event/on_subscribe_msg_status" ||
		nodeType == "event/on_group_manage" ||
		nodeType == "event/on_c2c_manage"
}

func shouldExecute(node *NodeData, ctx *EventContext) bool {
	switch node.Type {
	case "event/on_group_message":
		return ctx.EventType == "group_at_message" || ctx.EventType == "group_message"
	case "event/on_c2c_message":
		return ctx.EventType == "c2c_message"
	case "event/on_guild_message":
		return ctx.EventType == "at_message" || ctx.EventType == "guild_message"
	case "event/on_direct_message":
		return ctx.EventType == "direct_message"
	case "event/on_c2c_friend":
		return ctx.EventType == "c2c_friend"
	case "event/on_interaction":
		return ctx.EventType == "interaction"
	case "event/on_subscribe_msg_status":
		return ctx.EventType == "subscribe_msg_status"
	case "event/on_group_manage":
		return ctx.EventType == "group_manage"
	case "event/on_c2c_manage":
		return ctx.EventType == "c2c_manage"
	case "event/on_command":
		prefix := "/"
		if p, ok := node.Properties["prefix"].(string); ok && p != "" {
			prefix = p
		}
		msg := cleanMessageWithContext(ctx, ctx.Message)
		if !strings.HasPrefix(msg, prefix) {
			return false
		}
		cmdName := ""
		if c, ok := node.Properties["command"].(string); ok {
			cmdName = c
		}
		if cmdName == "" {
			return true
		}
		parts := strings.Fields(strings.TrimPrefix(msg, prefix))
		return len(parts) > 0 && parts[0] == cmdName
	default:
		return false
	}
}

// ==================== 辅助函数 ====================

// isTruthy 判断一个值是否为"真值"。用于条件节点输出端口的下游触发判断。
// false、nil、0、空字符串、空数组、空 map 视为假，其余为真。
func isTruthy(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case int:
		return val != 0
	case int64:
		return val != 0
	case float64:
		return val != 0
	case string:
		return val != ""
	case []interface{}:
		return len(val) > 0
	case map[string]interface{}:
		return len(val) > 0
	default:
		return true
	}
}

func getFloatVal(data interface{}, key string) float64 {
	var v interface{}
	if m, ok := data.(map[string]interface{}); ok {
		v = m[key]
	} else {
		v = data
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	}
	return 0
}

func getStrVal(data interface{}, key string) string {
	var v interface{}
	if m, ok := data.(map[string]interface{}); ok {
		v = m[key]
	} else {
		v = data
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case bool:
		return strconv.FormatBool(val)
	}
	return ""
}

func getStrProp(node *NodeData, key string) string {
	if v, ok := node.Properties[key].(string); ok {
		return v
	}
	return ""
}

func getNumProp(node *NodeData, key string) float64 {
	switch v := node.Properties[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	}
	return 0
}

func getBoolProp(node *NodeData, key string) bool {
	if v, ok := node.Properties[key].(bool); ok {
		return v
	}
	return false
}

func getStrInput(inputs map[string]interface{}, key string) string {
	if v, ok := inputs[key].(string); ok {
		return v
	}
	return ""
}

func getNumInput(inputs map[string]interface{}, key string) float64 {
	switch v := inputs[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}

// ==================== 事件节点执行器 ====================

type OnGroupMessageExecutor struct{}

func (e *OnGroupMessageExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	mentionIDs, mentionNames := extractMentionIDs(ctx)
	senderNick := ""
	if data, ok := ctx.EventData.(*dto.WSGroupATMessageData); ok && data.Author != nil {
		senderNick = data.Author.Username
	} else if data, ok := ctx.EventData.(*dto.WSGroupMessageData); ok && data.Author != nil {
		senderNick = data.Author.Username
	}
	return map[string]interface{}{
		"消息文本":   ctx.Message,
		"发送者ID":   ctx.UserID,
		"发送者昵称": senderNick,
		"群聊ID":    ctx.GroupID,
		"消息ID":    ctx.MessageID,
		"所有@ID":   mentionIDs,
		"所有@昵称":  mentionNames,
		"event":    ctx.EventData,
	}, nil
}

type OnC2CMessageExecutor struct{}

func (e *OnC2CMessageExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	mentionIDs, mentionNames := extractMentionIDs(ctx)
	senderNick := ""
	if data, ok := ctx.EventData.(*dto.WSC2CMessageData); ok && data.Author != nil {
		senderNick = data.Author.Username
	}
	return map[string]interface{}{
		"消息文本":   ctx.Message,
		"发送者ID":   ctx.UserID,
		"发送者昵称": senderNick,
		"消息ID":    ctx.MessageID,
		"所有@ID":   mentionIDs,
		"所有@昵称":  mentionNames,
		"event":    ctx.EventData,
	}, nil
}

type OnGuildMessageExecutor struct{}

func (e *OnGuildMessageExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	mentionIDs, mentionNames := extractMentionIDs(ctx)
	senderNick := ""
	if data, ok := ctx.EventData.(*dto.WSATMessageData); ok && data.Author != nil {
		senderNick = data.Author.Username
	}
	return map[string]interface{}{
		"消息文本":   ctx.Message,
		"发送者ID":   ctx.UserID,
		"发送者昵称": senderNick,
		"频道ID":    ctx.ChannelID,
		"GuildID":  ctx.GuildID,
		"消息ID":    ctx.MessageID,
		"所有@ID":   mentionIDs,
		"所有@昵称":  mentionNames,
		"event":    ctx.EventData,
	}, nil
}

type OnDirectMessageExecutor struct{}

func (e *OnDirectMessageExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	mentionIDs, mentionNames := extractMentionIDs(ctx)
	senderNick := ""
	if data, ok := ctx.EventData.(*dto.WSDirectMessageData); ok && data.Author != nil {
		senderNick = data.Author.Username
	}
	return map[string]interface{}{
		"消息文本":   ctx.Message,
		"发送者ID":   ctx.UserID,
		"发送者昵称": senderNick,
		"频道ID":    ctx.ChannelID,
		"GuildID":  ctx.GuildID,
		"消息ID":    ctx.MessageID,
		"所有@ID":   mentionIDs,
		"所有@昵称":  mentionNames,
		"event":    ctx.EventData,
	}, nil
}

type OnCommandExecutor struct{}

func (e *OnCommandExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	prefix := getStrProp(node, "prefix")
	if prefix == "" {
		prefix = "/"
	}
	cleanedMsg := cleanMessageWithContext(ctx, ctx.Message)
	text := strings.TrimPrefix(cleanedMsg, prefix)
	text = strings.TrimLeft(text, spaceChar)
	parts := strings.Fields(text)
	cmdName := ""
	if len(parts) > 0 {
		cmdName = parts[0]
	}
	var args []string
	argsText := ""
	if len(parts) > 1 {
		args = parts[1:]
		argsText = strings.Join(args, " ")
	}
	sourceID := ctx.GroupID
	if sourceID == "" {
		sourceID = ctx.UserID
	}
	mentionIDs, mentionNames := extractMentionIDs(ctx)
	firstMentionID := ""
	firstMentionName := ""
	if len(mentionIDs) > 0 {
		firstMentionID = mentionIDs[0]
	}
	if len(mentionNames) > 0 {
		firstMentionName = mentionNames[0]
	}
	return map[string]interface{}{
		"指令名":      cmdName,
		"参数列表":     args,
		"参数文本":     argsText,
		"清洗后消息":   cleanedMsg,
		"原始消息":     ctx.Message,
		"发送者ID":     ctx.UserID,
		"来源ID":      sourceID,
		"消息ID":      ctx.MessageID,
		"第一个@的ID":  firstMentionID,
		"第一个@的昵称": firstMentionName,
		"所有@ID":     mentionIDs,
		"所有@昵称":    mentionNames,
	}, nil
}

type OnC2CFriendExecutor struct{}

func (e *OnC2CFriendExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"事件数据": ctx.EventData,
	}
	if data, ok := ctx.EventData.(*dto.WSC2CFriendData); ok {
		result["用户OpenID"] = data.OpenID
		result["昵称"] = data.Nick
		result["时间戳"] = data.Timestamp
	}
	return result, nil
}

type OnInteractionExecutor struct{}

func (e *OnInteractionExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"事件数据": ctx.EventData,
	}
	if data, ok := ctx.EventData.(*dto.WSInteractionData); ok {
		result["互动类型"] = fmt.Sprintf("%d", data.Type)
		result["GuildID"] = data.GuildID
		result["ChannelID"] = data.ChannelID
	}
	return result, nil
}

type OnSubscribeMsgStatusExecutor struct{}

func (e *OnSubscribeMsgStatusExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"事件数据": ctx.EventData,
	}
	if data, ok := ctx.EventData.(*dto.WSSubscribeMsgStatus); ok {
		result["群OpenID"] = data.GroupOpenid
		result["用户OpenID"] = data.Openid
		result["授权结果"] = data.Result
	}
	return result, nil
}

type OnGroupManageExecutor struct{}

func (e *OnGroupManageExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"事件数据": ctx.EventData,
	}
	eventType := ""
	if v, ok := ctx.Variables["event_type"]; ok {
		eventType = fmt.Sprintf("%v", v)
	}
	result["事件类型"] = eventType
	if data, ok := ctx.EventData.(*dto.WSGroupManageData); ok {
		result["群OpenID"] = data.GroupOpenID
		result["操作者OpenID"] = data.OpMemberOpenID
		result["时间戳"] = float64(data.Timestamp)
	}
	return result, nil
}

type OnC2CManageExecutor struct{}

func (e *OnC2CManageExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"事件数据": ctx.EventData,
	}
	eventType := ""
	if v, ok := ctx.Variables["event_type"]; ok {
		eventType = fmt.Sprintf("%v", v)
	}
	result["事件类型"] = eventType
	if data, ok := ctx.EventData.(*dto.WSC2CManageData); ok {
		result["用户OpenID"] = data.OpenID
		result["时间戳"] = float64(data.Timestamp)
	}
	return result, nil
}

// ==================== 发送消息节点执行器 ====================

type SendMessageExecutor struct{}

func (e *SendMessageExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	content := getStrInput(inputs, "回复内容")
	if content == "" {
		content = getStrProp(node, "template")
	}
	if content == "" {
		return map[string]interface{}{"完成": true}, nil
	}
	var err error
	if ctx.GroupID != "" && ctx.ReplyToGroup != nil {
		err = ctx.ReplyToGroup(content)
	} else if ctx.UserID != "" && ctx.ReplyToUser != nil {
		err = ctx.ReplyToUser(content)
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"完成": true}, nil
}

type SendGroupMessageExecutor struct{}

func (e *SendGroupMessageExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	content := getStrInput(inputs, "消息内容")
	if content == "" {
		content = getStrProp(node, "template")
	}
	if content == "" {
		return map[string]interface{}{"完成": true}, nil
	}
	if ctx.ReplyToGroup != nil {
		if err := ctx.ReplyToGroup(content); err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{"完成": true}, nil
}

type SendC2CMessageExecutor struct{}

func (e *SendC2CMessageExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	content := getStrInput(inputs, "消息内容")
	if content == "" {
		content = getStrProp(node, "template")
	}
	if content == "" {
		return map[string]interface{}{"完成": true}, nil
	}
	if ctx.ReplyToUser != nil {
		if err := ctx.ReplyToUser(content); err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{"完成": true}, nil
}

type SendImageExecutor struct{}

func (e *SendImageExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	imageURL := getStrInput(inputs, "图片URL")
	if imageURL == "" {
		imageURL = getStrProp(node, "image_url")
	}
	if imageURL == "" {
		text := getStrInput(inputs, "文本")
		if text == "" {
			text = getStrProp(node, "text")
		}
		if text == "" {
			return nil, fmt.Errorf("图片URL或文本不能为空")
		}
		if ctx.GroupID != "" && ctx.ReplyToGroup != nil {
			return nil, ctx.ReplyToGroup(text)
		} else if ctx.UserID != "" && ctx.ReplyToUser != nil {
			return nil, ctx.ReplyToUser(text)
		}
		return map[string]interface{}{"完成": true}, nil
	}
	if ctx.GroupID != "" && ctx.ReplyImageToGroup != nil {
		_ = ctx.ReplyImageToGroup(imageURL)
	} else if ctx.UserID != "" && ctx.ReplyImageToUser != nil {
		_ = ctx.ReplyImageToUser(imageURL)
	}
	return map[string]interface{}{"完成": true}, nil
}

type SendRichMediaExecutor struct{}

func (e *SendRichMediaExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本内容")
	if text == "" {
		text = getStrProp(node, "text")
	}
	imageURL := getStrInput(inputs, "图片URL")
	if imageURL == "" {
		imageURL = getStrProp(node, "image_url")
	}

	// 图片URL为空时，只发送文本消息
	if imageURL == "" {
		if ctx.GroupID != "" && ctx.ReplyToGroup != nil {
			_ = ctx.ReplyToGroup(text)
		} else if ctx.UserID != "" && ctx.ReplyToUser != nil {
			_ = ctx.ReplyToUser(text)
		}
		return map[string]interface{}{"完成": true}, nil
	}

	// 有图片URL时发送图文消息
	if ctx.GroupID != "" && ctx.ReplyRichMediaToGroup != nil {
		_ = ctx.ReplyRichMediaToGroup(text, imageURL)
	} else if ctx.UserID != "" && ctx.ReplyRichMediaToUser != nil {
		_ = ctx.ReplyRichMediaToUser(text, imageURL)
	}
	return map[string]interface{}{"完成": true}, nil
}

type RecallMessageExecutor struct{}

func (e *RecallMessageExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	msgID := getStrInput(inputs, "消息ID")
	if msgID == "" {
		msgID = ctx.MessageID
	}
	return map[string]interface{}{"完成": true}, nil
}

// SendMarkdownExecutor 发送 Markdown 消息
type SendMarkdownExecutor struct{}

func (e *SendMarkdownExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	content := getStrInput(inputs, "Markdown内容")
	if content == "" {
		content = getStrProp(node, "content")
	}
	if content == "" {
		return nil, fmt.Errorf("Markdown内容不能为空")
	}
	var err error
	if ctx.GroupID != "" && ctx.ReplyMarkdownToGroup != nil {
		err = ctx.ReplyMarkdownToGroup(content)
	} else if ctx.UserID != "" && ctx.ReplyMarkdownToUser != nil {
		err = ctx.ReplyMarkdownToUser(content)
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"完成": true}, nil
}

// SendButtonsExecutor 发送带按钮的消息（Markdown 也可为空）
type SendButtonsExecutor struct{}

func (e *SendButtonsExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	markdown := getStrInput(inputs, "Markdown内容")
	if markdown == "" {
		markdown = getStrProp(node, "markdown")
	}
	buttonsJSON := getStrInput(inputs, "按钮配置")
	if buttonsJSON == "" {
		buttonsJSON = getStrProp(node, "buttons")
	}
	if buttonsJSON == "" {
		return nil, fmt.Errorf("按钮配置不能为空")
	}
	var err error
	if ctx.GroupID != "" && ctx.ReplyKeyboardToGroup != nil {
		err = ctx.ReplyKeyboardToGroup(markdown, buttonsJSON)
	} else if ctx.UserID != "" && ctx.ReplyKeyboardToUser != nil {
		err = ctx.ReplyKeyboardToUser(markdown, buttonsJSON)
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"完成": true}, nil
}

// SendMarkdownWithButtonsExecutor 发送 Markdown + 按钮
type SendMarkdownWithButtonsExecutor struct{}

func (e *SendMarkdownWithButtonsExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	markdown := getStrInput(inputs, "Markdown内容")
	if markdown == "" {
		markdown = getStrProp(node, "markdown")
	}
	buttonsJSON := getStrInput(inputs, "按钮配置")
	if buttonsJSON == "" {
		buttonsJSON = getStrProp(node, "buttons")
	}
	if markdown == "" {
		return nil, fmt.Errorf("Markdown内容不能为空")
	}
	if buttonsJSON == "" {
		return nil, fmt.Errorf("按钮配置不能为空")
	}
	var err error
	if ctx.GroupID != "" && ctx.ReplyKeyboardToGroup != nil {
		err = ctx.ReplyKeyboardToGroup(markdown, buttonsJSON)
	} else if ctx.UserID != "" && ctx.ReplyKeyboardToUser != nil {
		err = ctx.ReplyKeyboardToUser(markdown, buttonsJSON)
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"完成": true}, nil
}

// SendVideoExecutor 发送视频消息
type SendVideoExecutor struct{}

func (e *SendVideoExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	videoURL := getStrInput(inputs, "视频URL")
	if videoURL == "" {
		videoURL = getStrProp(node, "video_url")
	}
	if videoURL == "" {
		return nil, fmt.Errorf("视频URL不能为空")
	}
	var err error
	if ctx.GroupID != "" && ctx.ReplyVideoToGroup != nil {
		err = ctx.ReplyVideoToGroup(videoURL)
	} else if ctx.UserID != "" && ctx.ReplyVideoToUser != nil {
		err = ctx.ReplyVideoToUser(videoURL)
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"完成": true}, nil
}

// ==================== HTML 渲染节点执行器 ====================

// playwright 浏览器池：保持一个长驻浏览器进程，每次渲染创建新页面并行执行
var (
	pw          *playwright.Playwright
	pwBrowser   playwright.Browser
	pwOnce      sync.Once
	pwInitErr   error
	pwSem       chan struct{}
	pwMu        sync.Mutex
)

// initPlaywright 初始化全局浏览器池（只执行一次）
func initPlaywright() error {
	pwOnce.Do(func() {
		var err error
		pw, err = playwright.Run()
		if err != nil {
			pwInitErr = fmt.Errorf("启动 Playwright 失败: %w", err)
			return
		}

		pwBrowser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
			Headless: playwright.Bool(true),
			Args: []string{
				"--no-sandbox",
				"--disable-dev-shm-usage",
				"--disable-extensions",
				"--disable-plugins",
				"--disable-default-apps",
				"--disable-background-networking",
				"--disable-translate",
				"--disable-sync",
				"--disable-component-update",
				"--metrics-recording-only",
				"--safebrowsing-disable-auto-update",
			},
		})
		if err != nil {
			pwInitErr = fmt.Errorf("启动浏览器失败: %w", err)
			return
		}

		pwSem = make(chan struct{}, 4)
		fmt.Println("🌐 Playwright 浏览器池已初始化（并发上限: 4）")
	})
	return pwInitErr
}

// renderHTMLToImage 使用全局浏览器池渲染 HTML 为图片
func renderHTMLToImage(pageURL, htmlContent string, width float64) ([]byte, error) {
	if err := initPlaywright(); err != nil {
		return nil, err
	}

	pwSem <- struct{}{}
	defer func() { <-pwSem }()

	page, err := pwBrowser.NewPage()
	if err != nil {
		return nil, fmt.Errorf("创建页面失败: %w", err)
	}
	defer page.Close()

	if width > 0 {
		if err := page.SetViewportSize(int(width), 600); err != nil {
			return nil, fmt.Errorf("设置视口失败: %w", err)
		}
	}

	var buf []byte
	if pageURL != "" {
		_, err = page.Goto(pageURL, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			Timeout:   playwright.Float(30000),
		})
		if err != nil {
			return nil, fmt.Errorf("导航失败: %w", err)
		}

		time.Sleep(2 * time.Second)

		buf, err = page.Screenshot(playwright.PageScreenshotOptions{
			Type: playwright.ScreenshotTypePng,
		})
	} else {
		err = page.SetContent(htmlContent, playwright.PageSetContentOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			Timeout:   playwright.Float(30000),
		})
		if err != nil {
			return nil, fmt.Errorf("设置内容失败: %w", err)
		}

		time.Sleep(1 * time.Second)

		buf, err = page.Screenshot(playwright.PageScreenshotOptions{
			Type: playwright.ScreenshotTypePng,
		})
	}

	if err != nil {
		return nil, fmt.Errorf("渲染失败: %w", err)
	}
	return buf, nil
}

// HTMLToImageExecutor 将 HTML 字符串或 URL 渲染为图片并返回图片 URL
// 使用全局浏览器池，避免每次渲染都启动新进程
type HTMLToImageExecutor struct{}

func (e *HTMLToImageExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	htmlContent := getStrInput(inputs, "HTML内容")
	if htmlContent == "" {
		htmlContent = getStrProp(node, "html")
	}
	pageURL := getStrInput(inputs, "URL")
	if pageURL == "" {
		pageURL = getStrProp(node, "url")
	}
	width := getNumProp(node, "width")
	if width <= 0 {
		width = 800
	}

	if pageURL == "" && htmlContent == "" {
		return nil, fmt.Errorf("URL或HTML内容不能同时为空")
	}

	buf, err := renderHTMLToImage(pageURL, htmlContent, width)
	if err != nil {
		return nil, err
	}

	imageURL := fmt.Sprintf("data:image/png;base64,%s", encodeBase64(buf))
	return map[string]interface{}{"图片URL": imageURL}, nil
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// HTMLTemplateExecutor 使用内置模板生成 HTML
type HTMLTemplateExecutor struct{}

func (e *HTMLTemplateExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	templateName := getStrProp(node, "template_name")
	if templateName == "" {
		templateName = "card"
	}
	title := getStrInput(inputs, "标题")
	if title == "" {
		title = getStrProp(node, "title")
	}
	body := getStrInput(inputs, "正文")
	if body == "" {
		body = getStrProp(node, "body")
	}
	fontSize := getNumProp(node, "font_size")
	if fontSize <= 0 {
		fontSize = 16
	}
	fontColor := getStrProp(node, "font_color")
	if fontColor == "" {
		fontColor = "#333333"
	}
	bgColor := getStrProp(node, "bg_color")
	if bgColor == "" {
		bgColor = "#ffffff"
	}

	// 拼接 HTML 模板
	var html strings.Builder
	switch templateName {
	case "card":
		html.WriteString(`<div style="background:` + bgColor + `;padding:24px;border-radius:12px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;max-width:600px">`)
		if title != "" {
			html.WriteString(`<h2 style="color:` + fontColor + `;font-size:` + strconv.Itoa(int(fontSize)+4) + `px;margin:0 0 12px">` + title + `</h2>`)
		}
		html.WriteString(`<div style="color:` + fontColor + `;font-size:` + strconv.Itoa(int(fontSize)) + `px;line-height:1.6">` + body + `</div>`)
		html.WriteString(`</div>`)
	case "article":
		html.WriteString(`<div style="background:` + bgColor + `;padding:32px;font-family:Georgia,serif;max-width:700px">`)
		if title != "" {
			html.WriteString(`<h1 style="color:` + fontColor + `;font-size:` + strconv.Itoa(int(fontSize)+8) + `px;margin:0 0 16px;border-bottom:2px solid #eee;padding-bottom:8px">` + title + `</h1>`)
		}
		html.WriteString(`<div style="color:` + fontColor + `;font-size:` + strconv.Itoa(int(fontSize)) + `px;line-height:1.8">` + body + `</div>`)
		html.WriteString(`</div>`)
	case "notice":
		html.WriteString(`<div style="background:` + bgColor + `;padding:16px 24px;border-left:4px solid #2196F3;font-family:sans-serif;max-width:600px">`)
		if title != "" {
			html.WriteString(`<div style="color:#2196F3;font-size:` + strconv.Itoa(int(fontSize)) + `px;font-weight:bold;margin-bottom:8px">` + title + `</div>`)
		}
		html.WriteString(`<div style="color:` + fontColor + `;font-size:` + strconv.Itoa(int(fontSize)) + `px;line-height:1.5">` + body + `</div>`)
		html.WriteString(`</div>`)
	default:
		// 自定义模板：直接使用 body 作为 HTML
		html.WriteString(body)
	}

	return map[string]interface{}{"HTML内容": html.String()}, nil
}

// ==================== 文件操作节点执行器 ====================

// FileReadExecutor 读取本地文件内容
type FileReadExecutor struct{}

func (e *FileReadExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	path := getStrInput(inputs, "文件路径")
	if path == "" {
		path = getStrProp(node, "path")
	}
	if path == "" {
		return nil, fmt.Errorf("文件路径不能为空")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{"内容": "", "成功": false, "错误": err.Error()}, nil
	}
	return map[string]interface{}{"内容": string(data), "成功": true}, nil
}

// FileWriteExecutor 写入本地文件
type FileWriteExecutor struct{}

func (e *FileWriteExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	path := getStrInput(inputs, "文件路径")
	if path == "" {
		path = getStrProp(node, "path")
	}
	content := getStrInput(inputs, "内容")
	if content == "" {
		content = getStrProp(node, "content")
	}
	append := getBoolProp(node, "append")
	if path == "" {
		return nil, fmt.Errorf("文件路径不能为空")
	}

	flag := os.O_CREATE | os.O_WRONLY
	if append {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flag, 0644)
	if err != nil {
		return map[string]interface{}{"成功": false, "错误": err.Error()}, nil
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return map[string]interface{}{"成功": false, "错误": err.Error()}, nil
	}
	return map[string]interface{}{"成功": true}, nil
}

// ==================== 消息处理节点执行器 ====================

type ParseCommandExecutor struct{}

func (e *ParseCommandExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "消息文本")
	fromCtx := false
	if text == "" {
		text = ctx.Message
		fromCtx = true
	}
	prefix := getStrInput(inputs, "指令前缀")
	if prefix == "" {
		prefix = getStrProp(node, "prefix")
	}
	if prefix == "" {
		prefix = "/"
	}
	var cleaned string
	if fromCtx {
		cleaned = cleanMessageWithContext(ctx, text)
	} else {
		cleaned = cleanMessage(text)
	}
	trimmed := strings.TrimPrefix(cleaned, prefix)
	trimmed = strings.TrimLeft(trimmed, spaceChar)
	parts := strings.Fields(trimmed)
	cmdName := ""
	if len(parts) > 0 {
		cmdName = parts[0]
	}
	var args []string
	argsText := ""
	if len(parts) > 1 {
		args = parts[1:]
		argsText = strings.Join(args, " ")
	}
	return map[string]interface{}{
		"指令名":    cmdName,
		"参数列表":   args,
		"参数文本":   argsText,
		"清洗后文本": cleaned,
	}, nil
}

type ExtractMentionUserExecutor struct{}

func (e *ExtractMentionUserExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	mentionIDs, mentionNames := extractMentionIDs(ctx)
	firstName := ""
	firstID := ""
	if len(mentionNames) > 0 {
		firstName = mentionNames[0]
	}
	if len(mentionIDs) > 0 {
		firstID = mentionIDs[0]
	}
	return map[string]interface{}{
		"第一个@的昵称": firstName,
		"所有@昵称":    mentionNames,
		"第一个@的ID":  firstID,
		"所有@ID":     mentionIDs,
	}, nil
}

type ParseImagesExecutor struct{}

func (e *ParseImagesExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	attachments := extractAttachments(ctx)
	var imageURLs []string
	var imageFileNames []string
	var videoURLs []string
	var voiceURLs []string
	if attachments != nil {
		for _, a := range attachments {
			if a == nil {
				continue
			}
			if strings.HasPrefix(a.ContentType, "image/") {
				imageURLs = append(imageURLs, a.URL)
				imageFileNames = append(imageFileNames, a.FileName)
			} else if strings.HasPrefix(a.ContentType, "video/") {
				videoURLs = append(videoURLs, a.URL)
			} else if a.ContentType == "voice" {
				voiceURLs = append(voiceURLs, a.URL)
			}
		}
	}
	return map[string]interface{}{
		"图片URL列表":   imageURLs,
		"图片数量":     len(imageURLs),
		"第一张图片URL": firstOrEmpty(imageURLs),
		"图片文件名列表": imageFileNames,
		"视频URL列表":   videoURLs,
		"语音URL列表":   voiceURLs,
	}, nil
}

func firstOrEmpty(arr []string) string {
	if len(arr) > 0 {
		return arr[0]
	}
	return ""
}

// ==================== 条件判断节点执行器 ====================

type TextContainsExecutor struct{}

func (e *TextContainsExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	if text == "" {
		text = ctx.Message
	}
	keyword := getStrProp(node, "keyword")
	contains := keyword != "" && strings.Contains(text, keyword)
	return map[string]interface{}{"结果": contains}, nil
}

type TextEqualsExecutor struct{}

func (e *TextEqualsExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	if text == "" {
		text = ctx.Message
	}
	expected := getStrProp(node, "expected")
	equals := text == expected
	return map[string]interface{}{"结果": equals}, nil
}

type TextMatchesExecutor struct{}

func (e *TextMatchesExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	if text == "" {
		text = ctx.Message
	}
	pattern := getStrProp(node, "pattern")
	matched := false
	if pattern != "" {
		if re, err := regexp.Compile(pattern); err == nil {
			matched = re.MatchString(text)
		}
	}
	return map[string]interface{}{"结果": matched}, nil
}

type NumberCompareExecutor struct{}

func (e *NumberCompareExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	a := getNumInput(inputs, "数值A")
	b := getNumInput(inputs, "数值B")
	op := getStrProp(node, "operator")
	if op == "" {
		op = "=="
	}
	var result bool
	switch op {
	case "==":
		result = a == b
	case "!=":
		result = a != b
	case ">":
		result = a > b
	case ">=":
		result = a >= b
	case "<":
		result = a < b
	case "<=":
		result = a <= b
	}
	return map[string]interface{}{"结果": result, "成立": result, "不成立": !result}, nil
}

type IsEmptyExecutor struct{}

func (e *IsEmptyExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	val := inputs["值"]
	isEmpty := val == nil || val == ""
	if arr, ok := val.([]interface{}); ok && len(arr) == 0 {
		isEmpty = true
	}
	return map[string]interface{}{"结果": isEmpty, "值": val, "为空": isEmpty, "不为空": !isEmpty}, nil
}

// ==================== 逻辑节点执行器 ====================

type BranchExecutor struct{}

func (e *BranchExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	condition := false
	if v, ok := inputs["条件"].(bool); ok {
		condition = v
	}
	return map[string]interface{}{"条件成立": condition, "条件不成立": !condition}, nil
}

type AndExecutor struct{}

func (e *AndExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	a, _ := inputs["条件A"].(bool)
	b, _ := inputs["条件B"].(bool)
	return map[string]interface{}{"结果": a && b}, nil
}

type OrExecutor struct{}

func (e *OrExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	a, _ := inputs["条件A"].(bool)
	b, _ := inputs["条件B"].(bool)
	return map[string]interface{}{"结果": a || b}, nil
}

type NotExecutor struct{}

func (e *NotExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	v, _ := inputs["条件"].(bool)
	return map[string]interface{}{"结果": !v}, nil
}

// ==================== 文本处理节点执行器 ====================

type TemplateExecutor struct{}

func (e *TemplateExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	template := getStrProp(node, "template")
	result := template
	for k, v := range ctx.Variables {
		placeholder := "{" + k + "}"
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", v))
	}
	return map[string]interface{}{"结果": result}, nil
}

type ConcatExecutor struct{}

func (e *ConcatExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	a := getStrInput(inputs, "文本A")
	b := getStrInput(inputs, "文本B")
	sep := getStrProp(node, "separator")
	return map[string]interface{}{"结果": a + sep + b}, nil
}

type SubstringExecutor struct{}

func (e *SubstringExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	start := int(getNumProp(node, "start"))
	length := int(getNumProp(node, "length"))
	runes := []rune(text)
	if start < 0 {
		start = 0
	}
	if start > len(runes) {
		start = len(runes)
	}
	end := start + length
	if end > len(runes) {
		end = len(runes)
	}
	return map[string]interface{}{"结果": string(runes[start:end])}, nil
}

type ReplaceExecutor struct{}

func (e *ReplaceExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	search := getStrProp(node, "search")
	replace := getStrProp(node, "replace")
	if search != "" {
		text = strings.ReplaceAll(text, search, replace)
	}
	return map[string]interface{}{"结果": text}, nil
}

type SplitExecutor struct{}

func (e *SplitExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	sep := getStrProp(node, "separator")
	if sep == "" {
		sep = ","
	}
	parts := strings.Split(text, sep)
	return map[string]interface{}{"数组": parts, "长度": len(parts)}, nil
}

type TextLengthExecutor struct{}

func (e *TextLengthExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	return map[string]interface{}{"长度": len([]rune(text))}, nil
}

type ToUpperExecutor struct{}

func (e *ToUpperExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	return map[string]interface{}{"结果": strings.ToUpper(text)}, nil
}

type ToLowerExecutor struct{}

func (e *ToLowerExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	return map[string]interface{}{"结果": strings.ToLower(text)}, nil
}

type TrimExecutor struct{}

func (e *TrimExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	return map[string]interface{}{"结果": strings.TrimSpace(text)}, nil
}

type TextFindExecutor struct{}

func (e *TextFindExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	search := getStrInput(inputs, "查找内容")
	pos := -1
	if search != "" {
		pos = strings.Index(text, search)
	}
	return map[string]interface{}{"位置": pos}, nil
}

type TextCharAtExecutor struct{}

func (e *TextCharAtExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	index := int(getNumInput(inputs, "索引"))
	runes := []rune(text)
	char := ""
	if index >= 0 && index < len(runes) {
		char = string(runes[index])
	}
	return map[string]interface{}{"字符": char}, nil
}

type TextRegexReplaceExecutor struct{}

func (e *TextRegexReplaceExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	pattern := getStrProp(node, "pattern")
	replace := getStrProp(node, "replace")
	if pattern != "" {
		re, err := regexp.Compile(pattern)
		if err == nil {
			text = re.ReplaceAllString(text, replace)
		}
	}
	return map[string]interface{}{"结果": text}, nil
}

// ==================== 数学运算节点执行器 ====================

type MathNumberExecutor struct{}

func (e *MathNumberExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	val := getNumInput(inputs, "数值")
	// 如果没有输入，从节点 properties 的 value 字段读取
	if v, ok := node.Properties["value"]; ok {
		switch n := v.(type) {
		case float64:
			val = n
		case int:
			val = float64(n)
		case string:
			if f, err := strconv.ParseFloat(n, 64); err == nil {
				val = f
			}
		}
	}
	return map[string]interface{}{"数值": val}, nil
}

type TypeConvertExecutor struct{}

func (e *TypeConvertExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	targetType := "string"
	if t, ok := node.Properties["target_type"].(string); ok && t != "" {
		targetType = t
	}

	val := inputs["值"]
	if val == nil {
		val = ""
	}

	switch targetType {
	case "number":
		switch v := val.(type) {
		case float64:
			return map[string]interface{}{"结果": v}, nil
		case int:
			return map[string]interface{}{"结果": float64(v)}, nil
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return map[string]interface{}{"结果": f}, nil
			}
			return map[string]interface{}{"结果": float64(0)}, nil
		case bool:
			if v {
				return map[string]interface{}{"结果": float64(1)}, nil
			}
			return map[string]interface{}{"结果": float64(0)}, nil
		default:
			return map[string]interface{}{"结果": float64(0)}, nil
		}
	case "boolean":
		switch v := val.(type) {
		case bool:
			return map[string]interface{}{"结果": v}, nil
		case string:
			return map[string]interface{}{"结果": v != "" && v != "false" && v != "0"}, nil
		case float64:
			return map[string]interface{}{"结果": v != 0}, nil
		case int:
			return map[string]interface{}{"结果": v != 0}, nil
		default:
			return map[string]interface{}{"结果": false}, nil
		}
	default: // string
		return map[string]interface{}{"结果": fmt.Sprintf("%v", val)}, nil
	}
}

type MathAddExecutor struct{}

func (e *MathAddExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"A+B": getNumInput(inputs, "A") + getNumInput(inputs, "B")}, nil
}

type MathSubtractExecutor struct{}

func (e *MathSubtractExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"A-B": getNumInput(inputs, "A") - getNumInput(inputs, "B")}, nil
}

type MathMultiplyExecutor struct{}

func (e *MathMultiplyExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"A*B": getNumInput(inputs, "A") * getNumInput(inputs, "B")}, nil
}

type MathDivideExecutor struct{}

func (e *MathDivideExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	b := getNumInput(inputs, "B")
	if b == 0 {
		return map[string]interface{}{"A/B": float64(0)}, nil
	}
	return map[string]interface{}{"A/B": getNumInput(inputs, "A") / b}, nil
}

type MathModuloExecutor struct{}

func (e *MathModuloExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	b := getNumInput(inputs, "B")
	if b == 0 {
		return map[string]interface{}{"A%B": float64(0)}, nil
	}
	return map[string]interface{}{"A%B": math.Mod(getNumInput(inputs, "A"), b)}, nil
}

type MathRandomExecutor struct{}

func (e *MathRandomExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	min := getNumProp(node, "min")
	max := getNumProp(node, "max")
	if max <= min {
		max = min + 100
	}
	return map[string]interface{}{"随机数": math.Floor(min + rand.Float64()*(max-min+1))}, nil
}

type MathRoundExecutor struct{}

func (e *MathRoundExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"结果": math.Round(getNumInput(inputs, "数值"))}, nil
}

type MathFloorExecutor struct{}

func (e *MathFloorExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"结果": math.Floor(getNumInput(inputs, "数值"))}, nil
}

type MathCeilExecutor struct{}

func (e *MathCeilExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"结果": math.Ceil(getNumInput(inputs, "数值"))}, nil
}

type MathAbsExecutor struct{}

func (e *MathAbsExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"结果": math.Abs(getNumInput(inputs, "数值"))}, nil
}

type ToNumberExecutor struct{}

func (e *ToNumberExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	f, _ := strconv.ParseFloat(text, 64)
	return map[string]interface{}{"结果": f}, nil
}

type MathSqrtExecutor struct{}

func (e *MathSqrtExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"结果": math.Sqrt(math.Max(0, getNumInput(inputs, "数值")))}, nil
}

type MathSinExecutor struct{}

func (e *MathSinExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	deg := getNumInput(inputs, "角度")
	return map[string]interface{}{"结果": math.Sin(deg * math.Pi / 180)}, nil
}

type MathPiExecutor struct{}

func (e *MathPiExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"π": math.Pi}, nil
}

type MathIsEvenExecutor struct{}

func (e *MathIsEvenExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	n := int(getNumInput(inputs, "数值"))
	return map[string]interface{}{"结果": n%2 == 0}, nil
}

type MathSumExecutor struct{}

func (e *MathSumExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	var sum float64
	if arr, ok := inputs["列表"].([]interface{}); ok {
		for _, v := range arr {
			sum += getFloatVal(v, "")
		}
	}
	return map[string]interface{}{"总和": sum}, nil
}

type MathClampExecutor struct{}

func (e *MathClampExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	v := getNumInput(inputs, "数值")
	minVal := getNumInput(inputs, "最小值")
	maxVal := getNumInput(inputs, "最大值")
	return map[string]interface{}{"结果": math.Max(minVal, math.Min(maxVal, v))}, nil
}

type MathRandomFloatExecutor struct{}

func (e *MathRandomFloatExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	min := getNumProp(node, "min")
	max := getNumProp(node, "max")
	if max <= min {
		max = min + 1
	}
	return map[string]interface{}{"随机数": min + rand.Float64()*(max-min)}, nil
}

// ==================== 网络请求节点执行器 ====================

type HTTPRequestExecutor struct{}

func (e *HTTPRequestExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	method := getStrProp(node, "method")
	if method == "" {
		method = "GET"
	}
	reqURL := getStrInput(inputs, "URL")
	body := getStrInput(inputs, "请求体")

	var resp *http.Response
	var err error
	switch method {
	case "GET":
		resp, err = http.Get(reqURL)
	case "POST":
		resp, err = http.Post(reqURL, "application/json", strings.NewReader(body))
	default:
		resp, err = http.Get(reqURL)
	}
	if err != nil {
		return map[string]interface{}{"响应内容": "", "状态码": 0}, nil
	}
	defer resp.Body.Close()

	buf := make([]byte, 10240)
	n, _ := resp.Body.Read(buf)
	return map[string]interface{}{"响应内容": string(buf[:n]), "状态码": resp.StatusCode}, nil
}

type JSONParseExecutor struct{}

func (e *JSONParseExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "JSON文本")
	var obj interface{}
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		return map[string]interface{}{"解析结果": nil, "字段值": nil}, nil
	}
	field := getStrProp(node, "field")
	if field == "" {
		return map[string]interface{}{"解析结果": obj}, nil
	}
	val := obj
	for _, key := range strings.Split(field, ".") {
		if m, ok := val.(map[string]interface{}); ok {
			val = m[key]
		} else {
			val = nil
			break
		}
	}
	return map[string]interface{}{"解析结果": obj, "字段值": val}, nil
}

type JSONGenerateExecutor struct{}

func (e *JSONGenerateExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	obj := inputs["对象"]
	indent := getBoolProp(node, "indent")
	var data []byte
	var err error
	if indent {
		data, err = json.MarshalIndent(obj, "", "  ")
	} else {
		data, err = json.Marshal(obj)
	}
	if err != nil {
		return map[string]interface{}{"JSON文本": ""}, nil
	}
	return map[string]interface{}{"JSON文本": string(data)}, nil
}

type URLEncodeExecutor struct{}

func (e *URLEncodeExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	return map[string]interface{}{"结果": url.QueryEscape(text)}, nil
}

type URLDecodeExecutor struct{}

func (e *URLDecodeExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	decoded, err := url.QueryUnescape(text)
	if err != nil {
		decoded = text
	}
	return map[string]interface{}{"结果": decoded}, nil
}

// ==================== 变量节点执行器 ====================

type SetVariableExecutor struct{}

func (e *SetVariableExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	name := getStrProp(node, "name")
	value := inputs["值"]
	if name != "" {
		if ctx.Variables == nil {
			ctx.Variables = make(map[string]interface{})
		}
		ctx.Variables[name] = value
	}
	return map[string]interface{}{"完成": true}, nil
}

type GetVariableExecutor struct{}

func (e *GetVariableExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	name := getStrProp(node, "name")
	var value interface{}
	if ctx.Variables != nil {
		value = ctx.Variables[name]
	}
	return map[string]interface{}{"值": value}, nil
}

type DeleteVariableExecutor struct{}

func (e *DeleteVariableExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	name := getStrProp(node, "name")
	if ctx.Variables != nil {
		delete(ctx.Variables, name)
	}
	return map[string]interface{}{"完成": true}, nil
}

type ConstantExecutor struct{}

func (e *ConstantExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	value := getStrProp(node, "value")
	typeStr := getStrProp(node, "type")
	switch typeStr {
	case "number":
		var f float64
		fmt.Sscanf(value, "%f", &f)
		return map[string]interface{}{"值": f}, nil
	case "boolean":
		return map[string]interface{}{"值": value == "true"}, nil
	default:
		return map[string]interface{}{"值": value}, nil
	}
}

// ==================== 时间节点执行器 ====================

type TimeNowExecutor struct{}

func (e *TimeNowExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	now := time.Now()
	return map[string]interface{}{
		"时间戳(ms)": now.UnixMilli(),
		"日期时间":    now.Format("2006-01-02 15:04:05"),
	}, nil
}

type TimeFormatExecutor struct{}

func (e *TimeFormatExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	ts := getNumInput(inputs, "时间戳(ms)")
	if ts == 0 {
		ts = float64(time.Now().UnixMilli())
	}
	format := getStrProp(node, "format")
	if format == "" {
		format = "YYYY-MM-DD HH:mm:ss"
	}
	goFormat := format
	goFormat = strings.ReplaceAll(goFormat, "YYYY", "2006")
	goFormat = strings.ReplaceAll(goFormat, "MM", "01")
	goFormat = strings.ReplaceAll(goFormat, "DD", "02")
	goFormat = strings.ReplaceAll(goFormat, "HH", "15")
	goFormat = strings.ReplaceAll(goFormat, "mm", "04")
	goFormat = strings.ReplaceAll(goFormat, "ss", "05")
	t := time.UnixMilli(int64(ts))
	return map[string]interface{}{"格式化结果": t.Format(goFormat)}, nil
}

type TimeCalculateExecutor struct{}

func (e *TimeCalculateExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	ts := getNumInput(inputs, "时间戳(ms)")
	if ts == 0 {
		ts = float64(time.Now().UnixMilli())
	}
	amount := getNumProp(node, "amount")
	unit := getStrProp(node, "unit")
	operation := getStrProp(node, "operation")

	var ms float64
	switch unit {
	case "seconds":
		ms = 1000
	case "minutes":
		ms = 60000
	case "hours":
		ms = 3600000
	default:
		ms = 86400000
	}
	delta := amount * ms
	result := ts
	if operation == "subtract" {
		result = ts - delta
	} else {
		result = ts + delta
	}
	return map[string]interface{}{"结果时间戳": result}, nil
}

type DelayExecutor struct{}

func (e *DelayExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	delay := getNumProp(node, "delay")
	if delay <= 0 {
		delay = 1000
	}
	time.Sleep(time.Duration(delay) * time.Millisecond)
	return map[string]interface{}{"完成": true}, nil
}

// ==================== 日志节点执行器 ====================

type LogDebugExecutor struct{}

func (e *LogDebugExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	message := getStrInput(inputs, "消息")
	if message == "" {
		message = getStrProp(node, "message")
	}
	fmt.Printf("[Debug] %s\n", message)
	if ctx.LogDebug != nil {
		ctx.LogDebug(ctx.BotID, message)
	}
	return map[string]interface{}{"完成": true}, nil
}

// ==================== 时间节点执行器 ====================

type TimestampExecutor struct{}

func (e *TimestampExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	ts := time.Now().UnixMilli()
	return map[string]interface{}{"时间戳": ts}, nil
}

type TimeFormatCustomExecutor struct{}

func (e *TimeFormatCustomExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	ts := int64(getNumInput(inputs, "时间戳"))
	if ts <= 0 {
		ts = time.Now().UnixMilli()
	}
	format := getStrProp(node, "format")
	if format == "" {
		format = "2006-01-02 15:04:05"
	}
	t := time.UnixMilli(ts)
	return map[string]interface{}{"格式化时间": t.Format(format)}, nil
}

// ==================== 上下文信息节点执行器 ====================

type GetPluginIDExecutor struct{}

func (e *GetPluginIDExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"插件ID": ctx.BotID}, nil
}

// ==================== 列表节点执行器 ====================

var listStore = &ListStore{}

type ListCreateEmptyExecutor struct{}

func (e *ListCreateEmptyExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	listID := getStrProp(node, "list_id")
	if listID == "" {
		listID = fmt.Sprintf("list_%d", time.Now().UnixNano())
	}
	name := getStrProp(node, "name")
	if err := listStore.CreateList(listID, name, ctx.BotID); err != nil {
		return nil, err
	}
	return map[string]interface{}{"列表ID": listID}, nil
}

type ListCreateExecutor struct{}

func (e *ListCreateExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	listID := getStrProp(node, "list_id")
	if listID == "" {
		listID = fmt.Sprintf("list_%d", time.Now().UnixNano())
	}
	name := getStrProp(node, "name")
	content := getStrProp(node, "content")
	items := strings.Split(content, ",")
	for i := range items {
		items[i] = strings.TrimSpace(items[i])
	}
	if err := listStore.CreateList(listID, name, ctx.BotID); err != nil {
		return nil, err
	}
	if len(items) > 0 && items[0] != "" {
		if err := listStore.AddItems(listID, items); err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{"列表ID": listID}, nil
}

type ListAddRepeatExecutor struct{}

func (e *ListAddRepeatExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	listID := getStrProp(node, "list_id")
	if listID == "" {
		listID = getStrInput(inputs, "列表ID")
	}
	value := getStrProp(node, "value")
	count := int(getNumProp(node, "count"))
	if count <= 0 {
		count = 1
	}
	if err := listStore.AddItemRepeat(listID, value, count); err != nil {
		return nil, err
	}
	return map[string]interface{}{"完成": true}, nil
}

type ListLengthExecutor struct{}

func (e *ListLengthExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	listID := getStrProp(node, "list_id")
	if listID == "" {
		listID = getStrInput(inputs, "列表ID")
	}
	length, err := listStore.GetLength(listID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"长度": length}, nil
}

type ListIsEmptyExecutor struct{}

func (e *ListIsEmptyExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	listID := getStrProp(node, "list_id")
	if listID == "" {
		listID = getStrInput(inputs, "列表ID")
	}
	isEmpty, err := listStore.IsEmpty(listID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"结果": isEmpty, "为空": isEmpty, "不为空": !isEmpty}, nil
}

type ListFindExecutor struct{}

func (e *ListFindExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	listID := getStrProp(node, "list_id")
	if listID == "" {
		listID = getStrInput(inputs, "列表ID")
	}
	value := getStrInput(inputs, "查找内容")
	if value == "" {
		value = getStrProp(node, "value")
	}
	idx, err := listStore.FindIndex(listID, value)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"索引": idx}, nil
}

type ListGetExecutor struct{}

func (e *ListGetExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	listID := getStrProp(node, "list_id")
	if listID == "" {
		listID = getStrInput(inputs, "列表ID")
	}
	index := int(getNumInput(inputs, "索引"))
	value, err := listStore.GetItem(listID, index)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"值": value}, nil
}

type ListSetExecutor struct{}

func (e *ListSetExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	listID := getStrProp(node, "list_id")
	if listID == "" {
		listID = getStrInput(inputs, "列表ID")
	}
	index := int(getNumInput(inputs, "索引"))
	value := getStrInput(inputs, "值")
	if value == "" {
		value = getStrProp(node, "value")
	}
	if err := listStore.SetItem(listID, index, value); err != nil {
		return nil, err
	}
	return map[string]interface{}{"完成": true}, nil
}

type ListSubListExecutor struct{}

func (e *ListSubListExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	listID := getStrProp(node, "list_id")
	if listID == "" {
		listID = getStrInput(inputs, "列表ID")
	}
	start := int(getNumInput(inputs, "起始索引"))
	end := int(getNumInput(inputs, "结束索引"))
	items, err := listStore.GetSubList(listID, start, end)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"子列表": items}, nil
}

type ListFromTextExecutor struct{}

func (e *ListFromTextExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	text := getStrInput(inputs, "文本")
	if text == "" {
		text = getStrProp(node, "text")
	}
	delimiter := getStrProp(node, "delimiter")
	if delimiter == "" {
		delimiter = ","
	}
	items := strings.Split(text, delimiter)
	for i := range items {
		items[i] = strings.TrimSpace(items[i])
	}
	return map[string]interface{}{"列表": items}, nil
}

type ListSortExecutor struct{}

func (e *ListSortExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	listID := getStrProp(node, "list_id")
	if listID == "" {
		listID = getStrInput(inputs, "列表ID")
	}
	byNumber := getBoolProp(node, "by_number")
	ascending := getBoolProp(node, "ascending")
	if err := listStore.Sort(listID, byNumber, ascending); err != nil {
		return nil, err
	}
	return map[string]interface{}{"完成": true}, nil
}

type ListDeleteExecutor struct{}

func (e *ListDeleteExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	listID := getStrProp(node, "list_id")
	if listID == "" {
		listID = getStrInput(inputs, "列表ID")
	}
	if err := listStore.DeleteList(listID); err != nil {
		return nil, err
	}
	return map[string]interface{}{"完成": true}, nil
}

// ==================== 指令节点执行器 ====================

type CreateCommandExecutor struct{}

func (e *CreateCommandExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"完成": true}, nil
}

type PermissionCheckExecutor struct{}

func (e *PermissionCheckExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"结果": true, "有权限": true, "无权限": false}, nil
}

// ==================== 上下文信息节点执行器 ====================

type GetUserIDExecutor struct{}

func (e *GetUserIDExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"发送者ID": ctx.UserID}, nil
}

type GetGroupIDExecutor struct{}

func (e *GetGroupIDExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"群聊ID": ctx.GroupID}, nil
}

type GetChannelIDExecutor struct{}

func (e *GetChannelIDExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"频道ID": ctx.ChannelID}, nil
}

type GetGuildIDExecutor struct{}

func (e *GetGuildIDExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"GuildID": ctx.GuildID}, nil
}

type GetMessageIDExecutor struct{}

func (e *GetMessageIDExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"消息ID": ctx.MessageID}, nil
}

type GetSourceExecutor struct{}

func (e *GetSourceExecutor) Execute(ctx *EventContext, node *NodeData, inputs map[string]interface{}) (map[string]interface{}, error) {
	sourceType := "unknown"
	sourceID := ""
	if ctx.GroupID != "" {
		sourceType = "group"
		sourceID = ctx.GroupID
	} else if ctx.ChannelID != "" {
		sourceType = "guild"
		sourceID = ctx.ChannelID
	} else if ctx.UserID != "" {
		sourceType = "c2c"
		sourceID = ctx.UserID
	}
	return map[string]interface{}{"来源类型": sourceType, "来源ID": sourceID}, nil
}

// ==================== EventContext 构建 ====================

type ReplyCallbacks struct {
	ReplyText       func(content string) error
	ReplyImage      func(imageURL string) error
	ReplyRichMedia  func(text, imageURL string) error
	ReplyMarkdown   func(content string) error
	ReplyKeyboard   func(markdown, keyboardJSON string) error
	ReplyVideo      func(videoURL string) error
}

func ConvertWSDataToContext(botID string, eventType string, data interface{}, groupCallbacks *ReplyCallbacks, userCallbacks *ReplyCallbacks, logDebug func(botID, message string)) *EventContext {
	ctx := &EventContext{
		BotID:       botID,
		EventType:   eventType,
		EventData:   data,
		Variables:   make(map[string]interface{}),
		LogDebug:    logDebug,
	}

	switch v := data.(type) {
	case *dto.WSGroupATMessageData:
		ctx.Message = v.Content
		ctx.GroupID = v.GroupID
		ctx.ChannelID = v.ChannelID
		ctx.GuildID = v.GuildID
		ctx.MessageID = v.ID
		if v.Author != nil {
			ctx.UserID = v.Author.ID
		}
		if groupCallbacks != nil {
			ctx.ReplyToGroup = groupCallbacks.ReplyText
			ctx.ReplyImageToGroup = groupCallbacks.ReplyImage
			ctx.ReplyRichMediaToGroup = groupCallbacks.ReplyRichMedia
			ctx.ReplyMarkdownToGroup = groupCallbacks.ReplyMarkdown
			ctx.ReplyKeyboardToGroup = groupCallbacks.ReplyKeyboard
			ctx.ReplyVideoToGroup = groupCallbacks.ReplyVideo
		}
	case *dto.WSGroupMessageData:
		ctx.Message = v.Content
		ctx.GroupID = v.GroupID
		ctx.ChannelID = v.ChannelID
		ctx.GuildID = v.GuildID
		ctx.MessageID = v.ID
		if v.Author != nil {
			ctx.UserID = v.Author.ID
		}
		if groupCallbacks != nil {
			ctx.ReplyToGroup = groupCallbacks.ReplyText
			ctx.ReplyImageToGroup = groupCallbacks.ReplyImage
			ctx.ReplyRichMediaToGroup = groupCallbacks.ReplyRichMedia
			ctx.ReplyMarkdownToGroup = groupCallbacks.ReplyMarkdown
			ctx.ReplyKeyboardToGroup = groupCallbacks.ReplyKeyboard
			ctx.ReplyVideoToGroup = groupCallbacks.ReplyVideo
		}
	case *dto.WSC2CMessageData:
		ctx.Message = v.Content
		ctx.MessageID = v.ID
		if v.Author != nil {
			ctx.UserID = v.Author.ID
		}
		if userCallbacks != nil {
			ctx.ReplyToUser = userCallbacks.ReplyText
			ctx.ReplyImageToUser = userCallbacks.ReplyImage
			ctx.ReplyRichMediaToUser = userCallbacks.ReplyRichMedia
			ctx.ReplyMarkdownToUser = userCallbacks.ReplyMarkdown
			ctx.ReplyKeyboardToUser = userCallbacks.ReplyKeyboard
			ctx.ReplyVideoToUser = userCallbacks.ReplyVideo
		}
	case *dto.WSATMessageData:
		ctx.Message = v.Content
		ctx.ChannelID = v.ChannelID
		ctx.GuildID = v.GuildID
		ctx.MessageID = v.ID
		if v.Author != nil {
			ctx.UserID = v.Author.ID
		}
	case *dto.WSDirectMessageData:
		ctx.Message = v.Content
		ctx.ChannelID = v.ChannelID
		ctx.GuildID = v.GuildID
		ctx.MessageID = v.ID
		if v.Author != nil {
			ctx.UserID = v.Author.ID
		}
	case *dto.WSC2CFriendData:
		ctx.UserID = v.OpenID
	case *dto.WSSubscribeMsgStatus:
		ctx.GroupID = v.GroupOpenid
		ctx.UserID = v.Openid
	case *dto.WSInteractionData:
		ctx.GuildID = v.GuildID
		ctx.ChannelID = v.ChannelID
	case *dto.WSGroupManageData:
		ctx.GroupID = v.GroupOpenID
		ctx.UserID = v.OpMemberOpenID
	case *dto.WSC2CManageData:
		ctx.UserID = v.OpenID
	}

	return ctx
}
