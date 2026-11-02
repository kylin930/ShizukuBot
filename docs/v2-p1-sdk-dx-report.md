# P1.2 Go Plugin SDK 开发体验报告

## 基线与范围

开始时 `main` 工作区干净，执行 fetch 后本地与远程均为 `9cf1e69`。在其上创建 `feat/v2-p1-sdk-dx`，不修改、合并或推送 main。用户配置、插件状态和历史备份没有改写。

本轮只整理公共 SDK 和开发体验：不重写 EventBus、QQ transport、Manager 或 WebUI，不引入 Node、解释器、动态加载或进程外协议，go.mod/go.sum 没有新增依赖。

## 从 Hello 看变化

旧 Hello 需要结构体嵌入 Base、Manifest 的 API/runtime/权限/schema、New 工厂和 Load 注册。现在只需要 Meta、指令名和处理函数：

```go
package hello

import (
    "context"
    "shizukubot/sdk"
)

func Definition() sdk.Definition {
    return sdk.CommandPlugin(
        sdk.Meta{ID: "hello", Name: "Ping / Pong", Version: "1.0.0"},
        "ping", func(ctx context.Context, c sdk.Call) error {
            return c.Reply(ctx, "pong")
        },
    )
}
```

仓库实际 `plugins/hello/plugin.go` 包含注释共 **14 行**，将 Ping 保留为可直接单元测试的函数。开发者不需要手写结构体、工厂或空生命周期，仍有编译期类型检查和正常 error 返回。

## SDK 拆分与保留的底层机制

| 组成 | 本轮实现 |
| --- | --- |
| `sdk/definition.go` | Meta、Manifest、Definition、校验、当前 API major |
| `sdk/context.go` | Call、State、Reply/Send、ReplyMarkdown/ReplyImage |
| `sdk/lifecycle.go` | 原 Extension/Base/Host 方法集合 |
| `sdk/command.go`、`sdk/event.go`、`sdk/message.go` | 统一回调签名、规范事件常量/公共别名、原指令和 mention 解析 |
| `sdk/schema.go` | 移动原 schema 实现和方法，没有第二套配置模型 |
| `sdk/simple.go` | CommandPlugin、EventPlugin、Define 和两个明确 Options |
| `plugins/` | 原 Registry、Native、Manager、状态持久化、权限、异常和生命周期调度 |

公共依赖方向为 Plugin → SDK ← Runtime/Manager → EventBus。SDK 项目内部依赖实测只有 `events`，没有 Registry/Manager/QQ SDK。SDK 的 Event/EventType/Reply 是已有公共类型的别名，没有复制事件字段或 DTO。

Simple 构造器生成普通 Extension，Load 委托原 Host 注册，Start/HandleEvent/Stop/Unload 默认无操作；Manager 和 Native 的实例 gate、订阅、权限、配置/状态、取消、panic 捕获及错误记录继续复用。每个 New 是新对象，Define 的注册回调也对每个实例重新执行，没有新增运行时。

完整 Advanced 仍可显式定义 Manifest/New 并嵌入 sdk.Base，保留 Host.Context、配置、状态、后台任务和 Stop/Unload。当前 Native 适配器与 Manager 依旧在 plugins 包；不向 SDK 暴露静态 Registry 依赖，也没有为未来 RPC 预造通用宿主。

## API 决策

- **CommandPlugin**：自动当前 API major、runtime=go、空 object schema、commands.register 和 message.send。
- **EventPlugin**：自动 events.subscribe 和 message.send，支持多个规范类型并复制类型列表，避免调用方后续修改切片改变注册目标。
- **Define**：轻量的多指令/事件注册，默认两种注册和回复能力；初始化只做注册，长时间初始化或资源管理使用 Advanced。
- **WithConfig**：显式声明 schema，同时增加 config.read；**WithPermissions**：显式追加状态/日志等能力。没有默认配置、状态、日志或额外系统能力。
- **Meta**：保留 ID/Name/Version/Description/Author。Version 必须显式填写 major.minor.patch，可附 prerelease，没有自动默认版本。空白名称及无效 ID/版本返回明确字段错误。
- **错误时机**：构造器返回 Definition。Registry.Register 调用 Definition.Validate，在执行任何插件前验证元数据、schema、权限和工厂；指令/监听器的 nil handler、无效名称、空事件列表、重复命令等在 Load 返回错误，不用 panic 处理普通注册失败。
- **Registrar**：回调返回后封闭，保留首次注册错误；即使调用方误忽略某次错误，Load 仍失败。回调 panic 时也封闭 Registrar，panic 继续由原 Native 捕获。
- **回复**：两个新增媒体方法只调用既有 Send，保留能力和来源检查，透传原错误，没有添加无法发送的占位能力。当前频道/私信媒体仍返回 unsupported。
- **上下文**：通过 c.Event 访问唯一的 BotID/用户/群/内容来源，Args/Config/State 保持现有字段，不制造重复状态。

CommandHandler/EventHandler 使用同一函数签名的类型别名，既给文档明确名称，也保持旧 Host 方法签名和 Mock 的兼容性。Simple 不尝试复制用户函数捕获的可变对象：纯处理函数可直接复用；实例局部变量放在 Define 回调内，复杂资源放在 Advanced 结构体中。

## 示例、注册和兼容性

| 示例 | 修改前 | 修改后与行为 |
| --- | --- | --- |
| hello | 完整 Manifest/New/Load | CommandPlugin，仍 `/ping → pong`，保留直接函数测试 |
| autoreply | 结构体和手动遍历 On | EventPlugin + WithConfig，保留群/群 @/C2C、keyword/reply 及多 Bot 配置 |
| example/counter | 每次处理时读取磁盘的函数 | Advanced：Load 注册绑定方法，Start 恢复配置/状态到每 Bot 对象，写入成功才更新缓存，Stop/Unload 清理 |
| framework | 完整原生扩展 | 只改为 import 新 SDK，命令和返回内容不变 |

计数器损坏状态在 Start 报错；原测试中的有效回复和损坏状态拒绝场景保留，新增写入失败不推进计数、停止后不可调用和新实例恢复计数的测试。没有为了简短删除高级能力。

原 `plugins.Manifest/Definition/Extension/Base/Host/Call/State/Schema/APIVersion` 保留 deprecated 别名；ValidID/ParseCommand/CommandText 转发到 SDK。旧源码和原 P0/P1 测试继续编译，只有一份数据模型/解析实现。新代码和示例统一依赖 sdk。JSON 字段、持久数据、API 路由、配置和状态限额不变。

`go generate ./pluginregistry` 继续按插件路径确定排序，产出静态 imports；生成代码明确使用 plugins.Registry，接收 Simple/Advanced 的 sdk.Definition。重复 ID 返回错误。新增测试会生成、实际编译并运行混合 Simple/Advanced 的临时目录，再验证重复 ID 拒绝。测试最初因夹具缺少模块内 Registry 入口而失败，补齐委托真实 Registry 的入口后通过；没有改变校验逻辑或跳过测试。

离线迁移工具只调整新生成包的 SDK import，保留原语义分析/备份/回滚；旧生成源码仍通过别名编译。没有执行用户数据迁移或删除旧数据。

## 实际测试结果

Linux amd64、Go 1.26.10。先完成 SDK、示例、Registry、core/API、migration 定向验证，再做完整测试。所有最终测试均实际执行，未使用上一阶段数字代替。

| 检查 | 实际结果 |
| --- | --- |
| `go generate ./pluginregistry` | 通过；现有确定性测试和新增实际编译/重复 ID 测试通过 |
| `go test -count=1 -json ./...`，Go-only PATH/CGO_ENABLED=0 | **71 项顶层测试通过，0 失败、0 测试跳过**；14 个有测试包通过，4 个包无测试文件 |
| `go test -race -count=1 -json ./...` | 同样 **71 项通过**，没有 race 报告 |
| `go vet ./...` | 通过 |
| Linux 主程序、HTTP 工具构建 | Go-only PATH 构建通过 |
| Windows amd64、CGO_ENABLED=0 交叉构建 | 通过；未在 Windows 实机运行 |
| SDK 依赖检查 | 项目依赖仅 sdk/events，无 Manager、Registry、QQ SDK 或额外运行时 |
| SDK 新增场景 | 默认 Manifest、错误 Meta、自动生命周期、多事件、额外权限、注册错误、封闭表、独立闭包、回复透传通过 |
| 真实 Manager 新增场景 | Simple 多 Bot、启停/重启、事件/配置隔离、权限拒绝、Simple panic 与 Advanced 共存、错误状态、沙盒临时状态通过 |
| P0/P1 回归 | 多 Bot/global 回调隔离、去重、重连、取消、生命周期/资源、旧导入兼容、schema/状态/备份、认证/CSRF/WS 和沙盒等全部继续通过 |
| 媒体来源限制 | SDK helper 经过真实 core adapter，频道/私信拒绝不支持媒体且不发 QQ 请求；能力错误透传和发送权限拒绝通过 |
| 真实 HTTP 冒烟 | 登录、管理 API、普通静态 JS、脱敏、LiteGraph 路由 404、正常退出通过 |
| 实际沙盒 HTTP 并发 | 10 并发、100 请求：**100 成功、0 失败**；使用新的 Simple Hello，不安装代码、不修改插件生产数据 |
| gofmt、`git diff --check` | 通过，仅格式化本轮修改的 Go 文件 |

主要命令（Go 路径按实际安装替换）：

```sh
export PATH=/workspace/.tools/go1.26.10/bin:$PATH
export GOCACHE=/tmp/shizuku-go-cache GOTOOLCHAIN=local
go generate ./pluginregistry
go test -count=1 -json ./...
go test -race -count=1 -json ./...
go vet ./...
CGO_ENABLED=0 go build -buildvcs=false -o /tmp/shizukubot-sdk-dx .
CGO_ENABLED=0 go build -buildvcs=false -o /tmp/shizuku-http-sdk-dx ./cmd/concurrency-test
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -o /tmp/shizukubot-sdk-dx.exe .
go list -deps shizukubot/sdk
# 已有可选 HTTP 冒烟辅助脚本需要 Python，后端及插件构建/Go 测试不需要：
python3 scripts/p0-smoke.py --binary /tmp/shizukubot-sdk-dx --runner /tmp/shizuku-http-sdk-dx
```

Go-only 验证实际将 PATH 限为 `/workspace/.tools/go1.26.10/bin`，其中没有 Node/npm；测试里的源码编译也只调用 Go。`-buildvcs=false` 用于该受限 PATH 下避免额外 Git 元数据工具，不改变插件构建。竞态检查使用包含 C 编译器的常规 PATH。

机器可读测试日志：`/tmp/shizuku-sdk-dx-go-only.json`、`/tmp/shizuku-sdk-dx-race.json`；公共依赖清单 `/tmp/shizuku-sdk-dx-public-deps.txt`。实际冒烟目录 `/tmp/shizuku-p0-88_ak7xe` 含随机本地测试凭据，不应上传。

## 已知限制与未验证范围

- SDK 权限约束框架能力，不能阻止可信同进程 Go 代码自行读写文件/网络、调用 os.Exit、启动进程或耗尽资源。回调 recover 不捕获插件独立 goroutine panic；不合作代码仍可能阻塞停止，合作取消语义保持。
- Simple 不能为用户共享闭包/全局变量提供自动深复制；不要在多个 Bot 共享可变状态。Define 在实例初始化内部创建状态，或使用 Advanced/Call.State。
- 未连接真实 QQ、验证授权媒体、生产 HTTPS 或浏览器交互；使用 Mock、捕获回复和真实本机 HTTP 验证。Windows 只交叉构建，未声明实机运行通过。
- Schema 仍是受限方言，未做复杂状态版本迁移、跨 Bot 事务或 exactly-once；counter 的缓存以生命周期启动快照为准，运行中不支持外部直接改写状态文件。
- 兼容别名是渐进迁移入口，不维护第二套模型；新插件应尽早改用 sdk，第三方源码变更后仍需完整重编译和部署。
- 本轮没有 `.so`、进程外插件、RPC/IPC、热编译/热更新/文件监听或自动下载安装；未来进程外接口是否能复用当前同步调用需要单独设计，没有提前承诺无修改兼容。
