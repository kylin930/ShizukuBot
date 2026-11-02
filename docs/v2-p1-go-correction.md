# P1 架构纠偏：回归纯 Go

> 历史实现说明：本文件保留当时的 SDK 路径与验证事实。当前编译依赖已迁移到独立 botgo-plus，根目录 botgo-master 已移除；见 [P2.3.1 迁移报告](v2-p23-botgo-plus-migration-report.md)。


## 审查基线与最小方案

开始时分支 `feat/v2-p1-plugins`，HEAD 为 `7a02620`，工作区干净。在其上创建 `feat/v2-p1-go-correction`，没有重置原分支、覆盖 main、推送或发布。原 P1 的四个提交及用户数据保留。

审查发现：通用 Go 生命周期、配置/状态和 EventBus 已可用，但 Manager 仍通过 Store/Factory 在 Go 与脚本运行时之间选择，WebUI/API 以 ZIP 安装和信任开关为中心，迁移工具输出脚本。这些耦合是纠偏的主要修改范围，无需重写机器人核心或前端。

| 现有 P1 成果 | 本次处理 |
| --- | --- |
| events/Bus、Dispatcher 去重、transport/qq、多 Bot 路由 | 保留，核心和 SDK 无新增依赖 |
| API 配置快照/锁粒度、核心发送 context、内置指令优先级 | 保留有效实现和测试 |
| plugins 的 Manifest/Schema、Extension/Host、Native、State | 保留 Go 实现，补 Registry、初始化能力和回复便捷方法 |
| Manager 的实例 gate、按 Bot 配置/状态、订阅和清理 | 保留并简化为单一 Go 执行路径 |
| 捕获回复沙盒、管理员认证/CSRF/日志/统计、MDUI 2 | 保留，沙盒使用相同 Go 路由和临时状态 |
| Node Worker、进程监督、JSON-lines IPC、版本协商 | 删除 plugins/worker.go 与 plugins/worker/；没有独立 Go 用途，不保留抽象 |
| JS/TS SDK、TS 工具链、脚本示例 | 删除 sdk/、examples/plugins/、package.json、package-lock.json、.node-version |
| ZIP 脚本 Store 与安装/卸载/信任 UI/API | 删除 Store；运行时代码安装/卸载返回 410；管理已编译插件 |
| 代码 ZIP/私有备份 | 改为纯配置/状态 JSON 备份恢复，不能安装或执行代码 |
| 离线图解析、精确备份、能力检测、摘要 journal/回滚 | 保留；简单链输出真实 Go 包，复杂图仅报告人工迁移 |
| 脚本专属 API、协议、崩溃重建与包路径测试 | 随对应实现移除；通用隔离/生命周期/权限/配置/状态/沙盒断言迁移到 Go 测试 |
| 普通浏览器 JavaScript、MDUI 2 | 保留，不属于脚本插件体系 |

没有新增第三方依赖，go.mod/go.sum 和内置 botgo-master 依赖保持不变。旧 plugin/、LiteGraph 编辑器、图运行引擎与相关生产路由保持删除状态，没有重新引入。

## 新的注册、编译和运行方式

`plugins.Definition` = Manifest + `func() Extension`；统一 `Registry.Register` 拒绝重复/无效定义，Manager 在创建时取得目录快照。每个插件/Bot 及每次沙盒都由工厂创建新对象，不共享可变插件单例。

插件是普通 Go package：

- `plugins/hello`：/ping → pong。
- `plugins/autoreply`：群/C2C 关键词，独立 keyword/reply 配置。
- `plugins/example`：/count，独立 prefix 配置和持久计数。
- `plugins/framework`：保留原有 /framework Go 信息扩展。

每个包导出 `Definition()`，执行 `go generate ./pluginregistry`，由标准库源码解析器生成确定性静态 imports 和注册调用。生成阶段不运行插件，也不安装依赖。生成文件提交版本控制；只需 Go 工具链即可生成、测试和构建。

源码变更流程：审核 → 生成注册表 → 测试 → 构建 → 部署并重启主程序。接受重新编译的取舍，不实现任意 Go 源码热加载、解释器、`.so` 或进程外协议。未来确有外部插件安装需求再单独评估纯 Go 进程协议。

WebUI 可管理已编译插件的启停、配置、状态/错误、数据备份和沙盒。启停不重新编译；保存配置会重启已启用实例以应用快照。运行时上传源码或 ZIP 不会生效；旧安装和删除请求明确返回 410。配置导出现在是私有 JSON 数据，不能当代码包使用，属于本次有意的 P1 API 调整。

插件 Load/Start 注册，之后封闭；Stop/Unload 释放资源。Host 提供 BotID、长期 Context、Config、State、Log、Command、On；Call 提供规范 Event、参数和 Reply/Send 等明确能力。权限检查保留，但不是同进程 OS 隔离。

本次还修复初始化/调试结束时的清理顺序：先取消实例生命周期，再等待插件 Stop/Unload；禁用能取消正在执行的初始化和事件。单实例串行，多个实例可并行。回调错误/panic 记录到状态并继续其他处理器，后续事件可恢复；不再模拟进程重建和重试退避。Go 默认 2 秒超时是合作取消。

## 数据保留与迁移

`data/plugin-data` 继续保存配置与状态，未知旧脚本 ID 不恢复执行；旧 trusted 字段不再控制编译代码。原 `data/code-plugins`、旧图和自定义节点不加载、不删除，脚本需人工改写。相同 ID 的新 Go 包必须核对旧配置/状态兼容性，不能假设所有旧数据自动适用。

离线迁移命令备份旧图、自定义节点和旧脚本项目。图的全部节点/连接能被证明为简单线性固定回复时才生成 `plugin.go`；生成包有真实 Definition 和 Go 事件处理代码，测试会实际编译执行。生成状态为 generated-go-source，不等于注册、构建或启用成功。复杂图和脚本项目保留原文并报告 manual / manual-go-rewrite，没有伪 Go 占位转换。

本环境实际运行 dry-run/apply：**0 个旧图、0 个旧脚本项目，0 个用户插件转换，0 个用户插件删除**。新验证备份：

`data/legacy-backups/20261101-125905-3913476831/`

此目录含 checksums.json/report.json，未加入 Git。真实旧数据为空，不能将测试夹具转换数量当成用户迁移结果。旧阶段已有备份也保留。完整配置/状态备份应另行离线保存 `data/plugin-data`；在线 JSON 恢复拒绝已有数据/实例并保持禁用。

## 实际验证

环境 Linux amd64，Go 1.26.10。先跑 SDK/Registry/API/migration 定向测试，修复发现的问题后完成以下最终检查。首次 HTTP/WS 测试被执行沙箱的本地监听限制阻止；在允许本地监听的环境权限下重跑通过，未跳过断言或伪报成功。

| 检查 | 实际结果 |
| --- | --- |
| `go test -race -count=1 -json ./...` | **56 项顶层测试通过，0 失败、0 测试跳过，无 race 报告**；13 个有测试包通过，4 个包无测试文件 |
| 仅 Go 在 PATH：`CGO_ENABLED=0 go test -count=1 -json ./...` | 同样 56 项通过，不可从 PATH 找到 Node/npm |
| 仅 Go 在 PATH：`go generate ./pluginregistry` | 成功，生成内容 SHA256 与已提交生成文件一致 |
| 仅 Go 在 PATH：Linux 主程序和 HTTP 工具 build | 成功；`-buildvcs=false` 仅省去外部 Git 元数据采集 |
| Windows amd64，CGO_ENABLED=0 build | 成功交叉构建，未在 Windows 主机执行 |
| `go vet ./...`、`git diff --check` | 通过 |
| 三个 Go 示例的独立单元测试与 Registry 集成 | 回复、事件、配置、状态、多 Bot 隔离通过 |
| 生命周期与错误隔离 | 重复启用/重启、初始化失败清理、在途初始化/事件取消、后台资源停止、关闭调试、错误/panic、超时报告通过 |
| 配置/状态/备份/沙盒 | 校验、权限、持久状态、配置重启、空 scope 恢复、生产/调试隔离、全部媒体捕获通过 |
| P0/P1 回归 | 多 Bot 回调隔离、消息身份去重、重连、签名、核心锁/取消、认证/CSRF/Cookie/WS 与退役路由断言通过 |
| 迁移 | 生成 Go 编译/执行、群/C2C/命令/条件/Unicode、ID 去重、复杂图拒绝、原样脚本备份、重复检测与回滚保护通过 |
| 实际 Linux 程序冒烟 | 启动、管理员登录、管理 API、静态 app.js、脱敏、LiteGraph 路由 404、SIGTERM 正常退出通过 |
| 实际 HTTP 沙盒并发 | 10 并发、100 请求，**100 成功、0 失败**；使用已编译 hello，不安装代码或修改插件数据 |
| 内置 SDK 三包 | dto、event、interaction/signature 测试通过 |
| server 独立 Go 模块 | 仅 Go 路径构建成功；未连接远程 OSS/认证 |
| 浏览器普通 JS | app.js/community.js 语法检查通过；这是可选开发检查，不是构建或运行依赖 |
| 依赖与源码审查 | go list -deps 无脚本/图执行依赖；无生产 Node 启动、IPC、TS 或 npm 构建文件；普通浏览器 JS 保留 |

最终测试日志：`/tmp/shizuku-go-correction-final-race.json`、`/tmp/shizuku-go-only-tests.json`；依赖清单：`/tmp/shizuku-go-correction-deps.txt`。实际冒烟目录 `/tmp/shizuku-p0-v92xego1` 包含临时本机随机测试凭据，不能上传。

可复现的主要命令（将工具路径换成自己的 Go 安装路径）：

```sh
export PATH=/workspace/.tools/go1.26.10/bin:$PATH
export GOCACHE=/tmp/shizuku-go-cache GOTOOLCHAIN=local
go generate ./pluginregistry
go test -race -count=1 -json ./...
go vet ./...
CGO_ENABLED=0 go build -o /tmp/shizukubot-go-only .
CGO_ENABLED=0 go build -o /tmp/shizuku-go-http ./cmd/concurrency-test
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/shizukubot-go-correction.exe .
(cd botgo-master && go test -count=1 ./dto ./event ./interaction/signature)
(cd server && CGO_ENABLED=0 go build -o /tmp/shizuku-market-go-only .)
# 可选的已有进程冒烟辅助脚本需要 Python；后端和插件生成/构建/Go 测试不需要它。
python3 scripts/p0-smoke.py --binary /tmp/shizukubot-go-only --runner /tmp/shizuku-go-http
```

无 Node 验证使用 `PATH=/workspace/.tools/go1.26.10/bin`，设置 CGO_ENABLED=0；Go 测试中的迁移编译器仅调用同一 Go 工具。已安装的历史本地 node_modules 缓存不参与任何生产路径或 Go 测试，未清除用户本地文件。

## 尚未解决的问题与边界

- 原生插件在主进程运行，回调 recover 不隔离 os.Exit、独立 goroutine panic、死循环或资源耗尽。不合作的 Go 插件仍可能阻塞事件/停止；这是当前可信静态插件的明确边界。调试沙盒隔离回复和状态，不约束插件自行访问文件/网络。
- 本轮未用真实 QQ 凭证连接或验证授权媒体调用，使用规范化夹具、Mock 和捕获回复。Windows 仅交叉编译；未验证真实 Windows 运行、浏览器交互、生产 HTTPS 或 CDN/远程市场服务。
- SDK TokenSource 首次认证/刷新自身没有 context 接口，仍依赖有界 HTTP 超时；未为纠偏叠加 SDK 更换。
- 旧复杂图、旧脚本必须人工重写；高级历史文件/网络/渲染/数据库节点没有自动替代层。旧市场不支持 Go 插件在线安装。
- 不提供跨 Bot 状态事务、持久 exactly-once、副作用回滚或通用状态版本迁移；在线备份是文件级快照，严格一致性需停止主机。
- Bot 连接停止不自动禁用其插件；长驻资源可通过插件禁用释放。插件作者必须正确实现资源清理和后台同步。
- 未实施完整 RBAC、规则编辑器、进程外协议或后续阶段的市场/依赖生态，未把这些延期内容表述为完成。

下一阶段应先补充真实 QQ/Windows/浏览器部署验证和状态升级约定，再按实际 Go 插件需求扩展类型化事件/媒体能力和简单规则配置。保持构建期注册，不为模拟动态语言加载引入额外宿主。
