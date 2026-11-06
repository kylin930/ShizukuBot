# Go Plugin SDK v1

## 从 Hello 开始

```go
func Definition() sdk.Definition {
    return sdk.CommandPlugin(
        sdk.Meta{ID: "hello", Name: "Ping / Pong", Version: "1.0.0"},
        "ping", func(ctx context.Context, c sdk.Call) error {
            return c.Reply(ctx, "pong")
        },
    )
}
```

完整 package/import、构建和独立测试见 [五分钟创建插件](plugin-development.md)。插件只需 import `shizukubot/sdk`，无需了解 Manager 或 EventBus。SDK 不依赖具体运行时、Registry、QQ DTO 或 LiteGraph。

## 事件、配置和状态

EventPlugin 接收 Meta、`[]sdk.EventType`、EventHandler 和可选 Options。规范常量包括 GroupMessage、GroupAtMessage、C2CMessage、GuildMessage、AtMessage、DirectMessage、Interaction、C2CFriend、SubscribeMessageStatus、GroupManage、C2CManage、AnyEvent；自定义字符串仍可转换成 EventType，原规范化规则不变。EventPlugin 按首次出现顺序去重，ValidEventType 拒绝空、超过 128 字节、非法 UTF-8 或含空白/控制字符的类型，自定义类型仍可使用。

CommandHandler 和 EventHandler 都是 `func(context.Context, Call) error` 的别名，保留编译期检查和原 Host 的方法签名，不隐藏 error。

WithConfig(schema) 设置 schema 并授予 config.read；WithPermissions(...) 明确追加需要的框架能力。当前权限为 events.subscribe、commands.register、message.send、message.recall、group.manage、config.read、state.read、state.write、log.write，未知权限拒绝。它们不是操作系统安全策略。

Call 只有一个规范 Event：BotID、Type、PlatformType、EventID、MessageID、Source、UserID、GroupID、ChannelID、GuildID、Message、Raw、PluginIDs、Reply 和 Act。Args 为命令参数，Config 为独立配置快照，State 为当前插件/Bot 存储，Log 为受检查日志能力。Raw 是不可信平台数据，不含鉴权头或核心对象。字段视为只读。

| 常用调用 | 行为 |
| --- | --- |
| `c.Reply(ctx,text)` | 当前事件的文本回复 |
| `c.Send(ctx,sdk.Reply{...})` | text、image、rich_media、markdown、keyboard、video；携带 Text/URL/Keyboard |
| `c.ReplyMarkdown(ctx,text)` | 透传 Markdown 回复 |
| `c.ReplyImage(ctx,url)` | 透传图片回复 |
| `c.ReplyAt(ctx,text,userID)` | 文本回复并 @ 指定用户；群聊自动改用 Markdown |
| `c.ReplyImageBytes(ctx,data)` | 发送内存图片（≤5 MiB），自动生成 data URL |
| `c.ReplyImageFile(ctx,path)` | 读取本地图片并发送 |
| `c.Recall(ctx)` / `c.RecallMessage(ctx,id)` | 撤回当前消息 / 指定消息；需 message.recall |
| `c.Mute(ctx,d)` / `c.MuteUser(ctx,id,d)` | 禁言当前用户 / 指定用户；需 group.manage |
| `c.Unmute(ctx)` / `c.UnmuteUser(ctx,id)` | 解除禁言；需 group.manage |
| `c.Kick(ctx,blacklist)` / `c.KickUser(ctx,id,blacklist)` | 踢出成员；需 group.manage |
| `c.State.Get(ctx,key)` | JSON RawMessage，缺失返回 nil；需 state.read |
| `c.State.Set(ctx,key,value)` | 合法 JSON；需 state.write |

所有发送均使用同一主机回复能力，检查 message.send 和来源支持范围。群/C2C 使用核心媒体实现，频道/私信当前只有文本，其他媒体返回明确 unsupported。Reply 不得保存到回调结束后使用，传播传入 context，不暴露 QQ 客户端或其他 Bot sender。

管理动作通过 Event.Act 转发，按消息来源路由：群聊支持撤回、成员禁言/解禁和踢出；C2C 仅支持撤回；频道支持撤回、成员禁言/解禁和踢出；频道私信仅支持撤回。群聊 @ 用户仅 Markdown 消息支持，`ReplyAt` 会自动处理；群聊二进制直传上限 5 MiB；踢人接口受 QQ 白名单限制，未获权限时返回平台错误。`sdk.MentionUserIDs(Event)` 可提取消息中被 @ 的用户 ID（兼容新 `<qqbot-at-user>` 与旧 `<@id>` 格式）。

Schema 增加兼容的 `description` 字段供后台显示说明；数组错误带具体索引。配置 schema 保留原方言和校验：object、string、boolean、number、integer、array、properties、required、default、enum、minimum、maximum、items；未知字段拒绝，非完整 JSON Schema。Configure 保存后重启实例生效。State 按插件/Bot 隔离，key 最长 128、单值 64KiB、每文件 1MiB，原子写入；没有跨实例事务。自行创建的 goroutine 必须同步复合读改写。

## 构造器和校验时机

| 构造器 | 默认权限 | 初始化行为 |
| --- | --- | --- |
| CommandPlugin(meta,name,handler,options...) | commands.register、message.send | Load 调用 Host.Command |
| EventPlugin(meta,types,handler,options...) | events.subscribe、message.send | Load 为各类型调用 Host.On |
| Define(meta,register,options...) | commands.register、events.subscribe、message.send | 每实例 Load 调用 `func(Registrar) error` |

三者默认 APIVersion=当前 SDK major、Runtime=go、ConfigSchema=空 object，不默认授予配置/状态/日志。Meta 包含 ID、Name、Version、Description、Author；Version 必须是显式 major.minor.patch（可附 prerelease），没有隐式默认值。ID 为 1–64 个字母/数字/下划线/短横线，首位为字母或数字；名称不能空白。

构造器返回 Definition，不因普通注册失败 panic。`Meta.Validate`、`Manifest.Validate`、`Definition.Validate` 可显式检查，Registry.Register 自动调用 Definition.Validate，**不会为了检查而构造/执行插件**。无效元数据/schema/权限/空工厂在此返回错误；无效指令、nil handler、空事件列表、重复命令或注册回调错误在 Load 中返回，由 Manager 记录状态并清理。

Registrar 在注册回调返回后封闭，不能继续修改表；它会保留首次注册错误，防止忽略返回值后误报成功。仍应显式返回每个错误。Native 在 Start 后保留原有封闭检查。

每次 New 都创建新 Extension，Define 注册回调每实例执行。CommandPlugin/EventPlugin 传入的处理函数应无共享可变外部状态；SDK 无法复制 Go 闭包捕获的对象。实例局部可变状态放在 Define 注册回调内部或 Advanced 结构体中，持久状态放在 Call.State。

## Advanced API 与生命周期

```go
type Definition struct {
    Manifest Manifest
    New func() Extension
}

type Extension interface {
    Load(context.Context, Host) error
    Start(context.Context) error
    HandleEvent(context.Context, Call) error
    Stop(context.Context) error
    Unload(context.Context) error
}
```

Advanced 保留全部 Manifest 字段：APIVersion、ID、Name、Version、Description、Author、Runtime、Permissions、ConfigSchema；需显式声明兼容性和权限。嵌入 `sdk.Base` 可获取无操作默认方法，覆盖需要的 hook。工厂必须返回新对象，不能共享可变单例。

Host 嵌入 Registrar，并提供 Context()、BotID()、Config()、State()、Log(string)，方法集合与之前相同。Host.Context 是长期生命周期；初始化 context 仅用于 Load/Start。禁用/关闭/初始化失败/沙盒结束时先取消生命周期，再 Stop/Unload。清理须允许部分初始化；后台任务必须自行同步、响应取消，并在 Stop 等待释放。

Simple 内部生成普通 Extension，仍经过同一 Native、Manager、实例 gate、EventBus 过滤、配置/状态/回复权限和 panic 捕获。一个事件可同时触发指令、事件回调与 HandleEvent；HandleEvent 只在注册匹配时调用。多插件同名指令可分别响应，核心内置命令生产中优先消费。

同实例串行，不同实例可并行；默认 2 秒超时是合作取消。错误/panic 不阻断其他回调，后续调用可恢复。回调 recover 不覆盖插件创建的独立 goroutine、os.Exit 或资源耗尽，同进程可信代码没有 OS 隔离。

`sdk.ParseCommand` 保留 `/` 前缀、Unicode 空白、引号/反斜杠、无 shell 展开的语义；未闭合引号不匹配。`sdk.CommandText` 保留已知机器人 mention 处理及回退规则，原始 Event.Message 不改写。

## 注册、兼容与管理 API

Simple/Advanced 都导出 `Definition() sdk.Definition`，`go generate ./pluginregistry` 仍生成静态注册表，按路径确定排序、检查重复 ID，无运行时发现或解释器。源码变化重新构建/部署，启停和配置不重新编译。

旧 `plugins.Manifest/Definition/Extension/Base/Host/Call/State/Schema` 等仅为 deprecated 的 SDK 别名，ParseCommand/CommandText 为转发函数；旧源码仍能编译，JSON 字段、现有存储和管理路由保持。新代码使用 sdk，不依赖 plugins 实现。SDK 不引用 Registry/Manager，也未增加进程外适配层。

以下 HTTP 接口仍使用原管理员认证与 CSRF 策略，行为保持：

| 方法与路径 | 行为 |
| --- | --- |
| GET `/api/plugins`，`/api/plugins/{id}?bot_id=...` | 已编译目录、元数据、状态、指定 Bot 配置 |
| PUT `/api/plugins/{id}` | `{bot_id,config,apply?:false}`；返回 saved/applied/restart_required，应用失败附 restart_error |
| POST `/api/plugins/{id}/toggle` | `{bot_id,enabled}` |
| POST `/api/plugins/{id}/restart` | `{bot_id}`，保留状态 |
| GET `/api/plugins/{id}/export` | 私有 JSON 配置/状态备份 |
| POST `/api/plugins/data/restore` | 4 MiB JSON，真实 Bot 的空数据 scope 恢复，保持禁用 |
| POST `/api/bots/{id}/sandbox/send` | `{type:"group"或"c2c",content,plugin_id?,independent?:false}`，保留 replies/results/errors/executions |

旧代码安装/卸载仍为 410，图相关路由仍为 404。沙盒为每次运行创建新实例、临时状态和捕获 Reply，不需要 QQ、不发送真实消息，但不限制可信插件自行访问系统资源。本轮不包含 `.so`、RPC/IPC、热加载或自动安装，也不承诺未来进程外协议可以无修改复用当前同步接口。


### P2.1 管理状态与调试事实

正式插件列表/详情接受 bot_id 时验证真实 Bot；启停、配置、重启、恢复也在服务端验证范围。PluginInfo 保留旧字段，增加 compiled、bot_id、bot_exists、bot_running、allowed、running、state、last_error、config_error、restart_required。running 表示活动实例，State 仍可为 error；与 Bot 连接和选择策略分别解释。config 返回当前保存配置补上 Schema 默认值。

PUT 的 apply 默认 false，保持旧客户端显式重启流程。apply=true 仅在活动实例的有效配置快照不一致时调用原 Manager.Restart；配置校验失败不更改已保存值或运行实例。保存成功但重启失败返回 saved=true、applied=false、restart_error，不能当成整个请求失败后盲目重复保存。restart_required 反映当前实际配置差异，不无条件为 true。相同插件/Bot 的管理变更串行，普通查询不被全局锁阻塞。

Debug 保留原 error-only 方法；运行时增加 DebugWithResult 返回 MatchKnown/Matched，由真实注册匹配逻辑得出。ExecutionResult 保留 completed/failed 的 status，增加 outcome（replied/no_reply/unmatched/failed/timeout/canceled）、matched（未知时省略）、reply_count、duration_ms。duration_ms 包含当前实例初始化、执行及清理，来自实际测量。处理器主动不回复是 no_reply，不能当成 unmatched。

沙盒响应增加 configuration_source、independent。independent=true 强制默认配置和临时状态，不读取生产启用列表，须显式选择插件；否则读取该标识已保存配置及默认值。显式测试不受生产启用或允许列表限制，不改变生产订阅。真实 Bot 的“全部”只运行已启用且策略允许的插件。旧字段和路径不变，原代码安装接口仍为 410。
