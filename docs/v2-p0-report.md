# ShizukuBot v2 P0 交付报告

## 基线和范围

- 工作分支：`feat/v2-p0-core`。开始时工作区干净，原分支为 `work`；本地 HEAD 和远端 main 均为 `e86178d97724d72ec0e4e552f67de3fcedc7f238`，未覆盖 main。
- 已审查 main、Bot/BotManager、旧引擎/存储/Yaegi/转换器、API/沙盒、Webhook、登录/会话、配置及内置 BotGo 的事件、会话和连接实现。
- Go 1.26.10 / Linux amd64 / CGO GCC；使用原 go.mod、go.sum 和本地 BotGo replace。没有增加第三方依赖，没有修改内置 SDK。
- 本轮只建立 P0 基础，不实现完整代码插件系统、JS/TS worker、规则编辑器或 RBAC。未修改或发布云环境配置，未推送远端。

## 实际修改与根因

### A / B / D：独立事件链路

`transport/qq` / `webhook` → `events.Event` → `events.Dispatcher` → `events.Bus` → 内置指令 / `plugin.LegacyAdapter`。

原来每个 Bot.Start 都向 SDK 的全局 `DefaultHandlers` 写入闭包，后启动的机器人覆盖前一个。现在连接通过实例闭包绑定 Bot，HTTP 通过 URL 的 BotID 和对应凭证验签；二者使用同一个 Normalize 和业务入口，不再调用 SDK 全局业务注册。核心和传输没有 plugin 包依赖，公共事件没有 GraphData/NodeData。

保留 SDK OpenAPI/DTO，替换应用使用的不可取消 local session manager。新 gateway 管理每个 Bot 的分片、鉴权、heartbeat、重连/恢复、无效会话重置、鉴权终止错误和取消；停止时关闭读取并等待 worker / 在途事件结束。SDK TokenSource 按需刷新，不启动原来的全局刷新辅助协程（它会输出 token，持续失败还会 panic）。SDK 日志通过现有 logger 接口屏蔽原始参数，应用保留不含凭证的错误提示。

Bus 支持按 BotID/类型过滤、多订阅、幂等取消订阅、context、逐处理器 panic/error 隔离和协作式超时。订阅顺序执行，同一事件的一个处理器失败不阻止后续处理器；不同发布可以并发。提供 Middleware 类型，未来优先级/指令策略可包装 Handler，本轮未加入调度框架。事件按值传递，订阅方必须视其切片/原始 JSON 为只读。

兼容层承担旧 DTO 和图上下文转换。插件错误/panic 按插件记录并继续下一插件，图缓存按内容摘要更新，避免同秒保存仍执行旧图。内嵌自定义节点按执行局部绑定，避免不同插件同类型节点覆盖。旧 Go 解释执行、延时和 HTTP 节点增加 context 支持。统计通过锁内快照读取；移除 Bot 时不持有管理器锁等待 worker，避免回调查找机器人时死锁。修改机器人凭证会重建实例，避免只改字段却继续使用旧 TokenSource。

### C：平台身份去重

- 消息使用 BotID、会话来源/目标、发送者和 MessageID；明确合并群 AT/普通消息、频道 AT/普通消息的同消息别名。
- 缺失 MessageID 时可回退到准确的 EventID + 原始事件类型；非消息事件亦用事件 ID。没有可靠 ID 就不猜测、不按内容去重。
- TTL 5 分钟，容量 10,000，惰性清理，最多每秒完整清理一次；无永久清理协程。容量满时淘汰最早过期项。
- 去重在 Dispatcher 上进入旧插件前完成，原 ExecutionEngine 入口也采用同一身份策略。
- 这是 TTL 内尽力的 at-most-once admission；不是持久化 exactly-once。失败后部分处理器可能已产生副作用，不自动重放整个事件。进程重启或容量淘汰可能再次处理旧推送。

### E：管理后台安全

- 去除 main 的 session secret 和首次管理员密码日志。新管理员初始密码仅写 `data/.admin-initial-password`，权限 0600，并加入忽略规则；现有用户不会重新生成或覆盖。
- Cookie 使用 HttpOnly、SameSite=Lax；实际 TLS 自动 Secure。反向代理终止 HTTPS 时在本地 `config/config.json` 设置 `webui.secure_cookies: true` 并重启。生产必须使用 HTTPS、保留正确 Host；不信任 X-Forwarded-*。
- Cookie 绑定当前密码哈希的摘要，密码变更或用户重建会使旧 cookie 失效；没有把原始密码哈希写入 cookie。旧版本会话升级后需要重新登录。
- 修改请求验证 Origin / Referer / Fetch Metadata；无来源头的客户端必须使用 application/json。去除通配 CORS。原生 JSON 客户端和同源登录保持可用。Webhook 使用独立签名验证，不依赖浏览器会话。
- API 和日志 WebSocket 增加管理员检查；日志 WS 校验 Origin，设置读写上限、ping/pong/超时、权限复查、断开检测及订阅释放。普通账号仍可查询自己的身份；不新增细粒度 RBAC。
- 登出改为 POST，现有按钮只调整请求方式，保留 MDUI 2 / LiteGraph 视觉和页面。
- 配置返回的 password 字段为空；AppSecret 不进入 BotInfo 或 Bot 的 JSON。空 session secret 更新值表示保留旧值。配置写入权限收紧为 0600，新配置的会话密钥增至 32 字符。
- API 请求体有大小限制，端口/来源类型/插件 ID 校验；插件存储通过 os.Root 防路径和符号链接逃逸，读写返回快照，避免共享可变指针。新插件 ID 使用随机值。
- 文件/目录源码扫描限制在 data 树，通过 os.Root 防止 symlink 逃逸；inline 转换保留。API 的扫描、发送、用户管理错误不再返回底层文件/SDK 错误。
- 管理 API 使用互斥锁保护共享配置操作；这不是高吞吐的请求调度器，长时间调试任务可能阻塞其他管理 API，后续可细化锁粒度。

### F：沙盒

仍用现有 `/api/bots/{id}/sandbox/send`；支持虚拟或已配置机器人身份、群聊/C2C、指定旧插件。回复保持原 `replies`，新增类型化 `results`、`errors` 和逐插件 `executions`（ID、matched、status）。

沙盒使用单独 Bus 和捕获型 Reply，不进入生产 Dispatcher/订阅，不取得 Bot 发送函数。文本、图片、富媒体、Markdown、按钮、视频都被捕获；旧 `ProcessEventAndGetRepliesWithPlugin` 辅助方法也转到同一安全捕获路径。没有 QQ 连接即可测试，真实 API 发送能力不会被传给模拟事件。

## 兼容性与迁移说明

- LiteGraph 前端、JSON 数据、存储、code_converter.go、已有节点、富媒体方法和内置指令保留，无插件数据删除。
- 群/C2C、频道/私信、互动、好友、订阅状态、群/C2C 管理事件保留；未知平台类型以原始类型和 JSON 进入新 Bus，旧插件不会被强行映射到错误事件。
- 标准 API 的原 JSON envelope 保留；沙盒仅增加字段。安全相关的有意变化：管理员授权、跨站请求限制、POST 登出、扫描路径限制、secret 不回显、旧 cookie 失效。
- 原来无效/不可读的插件文件仍不自动删除或改写；不安全 ID 的历史文件不会加载，需备份后人工规范命名。本轮不做破坏式数据迁移。
- 未改 go.mod/go.sum、botgo-master、litegraph.js-release 或 code_converter.go。

## botgo-plus 调查

只读检查 `kylin930/botgo-plus` main，提交 `de69850024052e6912925da22bd33bdb0d5c19d8`（仓库外 `/tmp` checkout，无依赖迁移）。

- 模块路径已改为 `github.com/kylin930/botgo-plus`，内部 import 也已相应更新，迁移需系统调整 import，不能仅替换 require 版本。
- 具有相近 OpenAPI/DTO 及 FileData 富媒体扩展，有后续评估价值。
- event 和 websocket client 仍使用全局 DefaultHandlers；sessions/local 仍无 context 生命周期，token 辅助函数及日志风险仍需独立处理。
- 当前本地扩展的 GROUP_MESSAGE_CREATE / GroupMessageEventHandler 在该提交中缺失，迁移必须补齐兼容清单。
- 结论：可以作为后续 SDK 候选，但不是本轮可直接替换的修复。未执行真实 QQ 兼容性认证，不承诺 drop-in。

## 实际验证

工作目录 `/workspace/ShizukuBot`：

```bash
export PATH=/workspace/.tools/go1.26.10/bin:$PATH
export GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOMAXPROCS=4

go test -count=1 -json ./...
go test -race -count=1 ./...
go test -race -count=1 ./api ./plugin

go build -p 4 -o /workspace/.tools/bin/shizukubot-p0 .
go build -p 4 -o /workspace/.tools/bin/shizuku-concurrency-p0 ./cmd/concurrency-test
python3 scripts/p0-smoke.py \
  --binary /workspace/.tools/bin/shizukubot-p0 \
  --runner /workspace/.tools/bin/shizuku-concurrency-p0
node --check webui/static/js/app.js
git diff --check

(cd botgo-master && go test -count=1 ./dto ./event ./interaction/signature)
```

结果：

| 检查 | 实际结果 |
| --- | --- |
| 最终根模块测试 | 25 项顶层测试通过，0 失败、0 跳过；7 个含测试的包，其余包显示无测试 |
| 竞态检测 | 完整根模块通过；最后适配层/后台修改后再次运行 api/plugin，均通过，无 race 报告 |
| 内置 SDK 定向测试 | dto、event、interaction/signature 三包通过 |
| 编译 | 主程序及现有并发测试工具均成功 |
| 实际程序冒烟 | 独立临时配置启动、登录、配置/Bot/统计接口、主页及 LiteGraph 资源通过 |
| 现有 HTTP 并发工具 | 10 并发、100 请求：100 成功、0 失败，测试插件清理成功 |
| 退出与日志 | SIGTERM 正常退出，生成的管理员密码和会话密钥未出现在启动/退出日志中 |
| JavaScript / 补丁 | node --check 与 git diff --check 通过 |

定向回归包含：双 Bot 不受 SDK 全局回调影响、启停/重启/失败取消、移除期间在途回调、Mock gateway identify→resume→取消、签名 chunked Webhook、多订阅错误与 panic、去重 TTL/容量/并发、合法同文不同 ID、无 ID、旧 JSON 重新加载和执行、插件错误/异常后继续、捕获型媒体回复、沙盒机器人/插件选择、登录/权限/CSRF/Cookie/密码变更撤销、WS Origin、扫描与 symlink 路径限制。

测试工具使用临时数据目录，未修改现有 config/config.json、用户或插件数据。机器可读测试输出保存在 `/tmp/shizuku-p0-final-tests.json`；冒烟工具输出临时产物路径，文件包含本地测试凭据，不应上传。

## 未完成验证与已知风险

1. 无真实 QQ 凭据，未做平台实连接、实际群/C2C/频道富媒体、分片配额、封禁响应或长时间运行验证。相关可测部分使用本地 WS/HTTP 服务、静态 TokenSource 和捕获回复测试。生产 HTTPS/反向代理拓扑也未实测。
2. 旧插件仍在同进程运行，Yaegi 标准库、文件、HTTP、浏览器节点可访问宿主资源。沙盒是功能调试/回复捕获，不是安全执行隔离。它不会通过正常回复回调发送 QQ 消息，但恶意插件仍能自行联网；不能加载不可信插件。
3. context 是协作约束；Go/外部库不配合时不能强制抢占。正常 handler panic 被恢复，但任意原生代码 os.Exit、独立 goroutine panic、资源耗尽不能靠事件总线防护。Stop 等待在途工作，非协作插件可能拖延停止。
4. 去重不持久、有限容量，处理器部分失败不具备事务回滚；错误被返回，但不保证平台重试能重新执行已 admission 的事件。后续若需要重试，必须按处理器保存幂等状态。
5. 既有日志或旧 config 中曾泄露的凭据不会自动擦除；已有部署应轮换会话密钥/相关凭据并按自身策略处理旧日志。SDK 原始参数日志已屏蔽，排障细节减少属于明确的安全取舍。
6. 插件图的完整静态验证、文件写入原子化/崩溃恢复、精细授权、登录限流、分布式事件持久化不是本轮完成项。旧损坏图/文件不自动修复，发布前应备份并验证实际插件。
7. 没有运行需要真实凭据的 BotGo examples API 集成测试，没有启动独立 server/ 市场服务，也没有安装/验证 Playwright 截图运行时；这些不参与本轮核心 Mock 工作流的通过结论。

## P1 建议

1. 以 events/ 和 Reply 能力为基础定义版本化插件 SDK、manifest、配置和权限模型，先用少量纯代码插件验证生命周期。
2. 在独立进程中实现 JS/TS/Go 扩展协议、资源限制、取消与超时终止，明确网络/文件/发送能力；不直接开放 Yaegi 为所谓隔离沙盒。
3. 引入可选的处理器级幂等记录、失败队列和可观测性，再设计优先级/指令中间件与轻量规则编辑器。
4. 根据真实平台测试建立 SDK 兼容矩阵后，单独迁移 SDK；保留受测的事件入口与旧图适配器作为迁移边界。
