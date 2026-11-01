# Go Plugin SDK v1

公共类型位于 `plugins/model.go`、`plugins/catalog.go` 和 `events/events.go`，与 QQ SDK DTO、LiteGraph 和旧节点无关。

## 定义与注册

```go
type Definition struct {
    Manifest Manifest
    New func() Extension
}
```

Manifest 使用 API major 1，ID 为 1–64 个字母/数字/下划线/短横线（首位字母或数字），Version 为 semver，Runtime 为 `go`，另有 Name、Description、Author、Permissions、ConfigSchema。Registry.Register 校验并拒绝重复 ID；构造函数必须返回新的对象，禁止多 Bot 共用可变单例。Manager 构造时复制 Registry。

普通包暴露 `Definition()`，通过生成的 `pluginregistry.New()` 自动汇总；也可显式 `registry.Register(definition)`。`go generate ./pluginregistry` 后需测试、重新构建并部署。WebUI 不安装源码，修改源码后仅点“重启”不能替换二进制中的代码。

## 接口与生命周期

```go
type Extension interface {
    Load(context.Context, Host) error
    Start(context.Context) error
    HandleEvent(context.Context, Call) error
    Stop(context.Context) error
    Unload(context.Context) error
}
```

嵌入 `plugins.Base` 可获得无操作默认方法，只覆盖需要的 hook。Host 提供：

| 方法 | 用途 |
| --- | --- |
| Context() | 长期生命周期 context，禁用/关闭/调试结束或初始化失败时取消 |
| BotID() | 当前实例的 Bot 标识 |
| Config() | config.read 下的独立配置快照，无权限时为空 |
| State() | state.read/state.write 权限检查后的当前 Bot 状态 |
| Log(string) | log.write 下写入管理日志 |
| Command(name, handler) | 注册不带 `/` 的指令；需要 commands.register |
| On(eventType, handler) | 规范类型或 `*`；需要 events.subscribe |

Load/Start 注册后封闭，同实例重复命令拒绝。一个事件可同时触发指令、事件监听和 HandleEvent；HandleEvent 只在至少一个注册匹配时调用。多插件同名指令不会覆盖；核心内置命令在生产中优先消费。

初始化 context 只用于初始化，后台任务监听 Host.Context，并由 Stop 等待释放。Stop 和 Unload 在初始化失败时也可能调用，应允许部分初始化。事件、初始化和停止串行；回调须遵守 context。默认 2 秒超时只请求取消，不能抢占不合作的 Go 代码。回调 panic 转为错误，其他处理器继续执行；自行创建的 goroutine 要自行处理 panic 和同步。

## 事件上下文与能力

Call 包含 Event、Args、Config、State、Log。Event 提供 BotID、Type、PlatformType、EventID、MessageID、Source、UserID、GroupID、ChannelID、GuildID、Message、Raw 和 Bot 的插件选择。Raw 是不可信平台数据，应按需要验证；不含鉴权头或核心对象。上下文及事件字段视为只读。

`Call.Reply(ctx,text)` 发送文本，`Call.Send(ctx,events.Reply{...})` 发送其他类型。Kind 支持 text、image、rich_media、markdown、keyboard、video，可带 Text、URL、Keyboard。需要 message.send，绑定当前事件的来源。不能保存 Reply 并在回调结束后使用，须传播传入 context。群/C2C 使用核心已有媒体能力，频道/私信目前支持文本，其他媒体返回明确错误。

`ParseCommand` 支持 `/name`、Unicode 空白、单双引号和反斜杠转义，不作 shell 展开；未闭合引号不匹配。解析前通过 `CommandText` 移除已知机器人 mention；原始 Message 保留。其他前缀可使用 On 实现。

State.Get 返回 JSON RawMessage，缺失 key 返回 nil；Set 接收合法 JSON。按插件/Bot 隔离，key 最长 128，单值 64KiB、每文件 1MiB，原子持久化。同实例事件串行；插件自行创建的 goroutine 应自行同步复合读改写，不提供跨 Bot/插件事务。权限为 state.read / state.write。

ConfigSchema 支持 object、string、boolean、number、integer、array、properties、required、default、enum、minimum、maximum、items；未知键拒绝，非完整 JSON Schema。Configure 保存后重启生效。不要在插件配置放核心凭证。权限不会限制可信 Go 代码自行访问操作系统。

## 管理与沙盒 API

均使用原管理员认证和 CSRF 策略。

| 方法与路径 | 行为 |
| --- | --- |
| GET `/api/plugins`，`/api/plugins/{id}?bot_id=...` | 已编译目录、元数据、状态、指定 Bot 配置 |
| PUT `/api/plugins/{id}` | `{bot_id,config}`；返回 restart_required |
| POST `/api/plugins/{id}/toggle` | `{bot_id,enabled}`；无需信任开关或重新编译 |
| POST `/api/plugins/{id}/restart` | `{bot_id}`；清理并创建新实例，保留状态 |
| GET `/api/plugins/{id}/export` | 私有 JSON 配置/状态数据备份，不包含代码 |
| POST `/api/plugins/data/restore` | 原始 JSON 备份；仅已编译插件且无已有数据，恢复后禁用 |
| POST `/api/bots/{id}/sandbox/send` | `{type:"group"或"c2c",content,plugin_id?}`；返回 replies/results/errors/executions |

旧 POST `/api/plugins` 和 DELETE `/api/plugins/{id}` 返回 410 并说明构建部署流程。旧 graph/custom-nodes/code2graph/graph2code 路由保持 404。旧脚本 ZIP 不能通过数据恢复接口执行。

沙盒 Debug 总是使用新实例、临时状态和捕获回复，不需要真实 QQ Bot/连接。每次调试计数从头开始，不改生产状态。沙盒是功能测试工具，没有 OS 安全隔离；仅执行编译时审核过的 Go 插件。
