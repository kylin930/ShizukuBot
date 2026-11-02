# P2.3.1 botgo-plus SDK 统一维护与依赖迁移

日期：2026-11-02。此次交付不创建正式 Release/Tag，不合并任何 main，不发送真实 QQ 消息。

## 两个仓库与依赖基线

| 项目 | 基线 / 分支 |
| --- | --- |
| botgo-plus | main `de69850` → `feat/shizukubot-protocol-sync` |
| ShizukuBot | P2.3 `8f396398faebc73b2f42b51d7709f406aca36075` → `feat/v2-p23-botgo-plus-migration` |

工作前两个仓库工作区均干净并 fetch 远程。ShizukuBot main 仍为 `fc4db97`，尚未
包含 P2.3，故从 P2.3 功能分支派生，未覆盖或丢失上一轮实现。没有 reset 或强制推送。

最终依赖：

```go
require github.com/kylin930/botgo-plus v0.0.0-20261102144408-979b74884c88
```

对应不可变 SDK 实现/测试提交 **979b74884c88f5f0dfa21a5c2d8315b6ec654f4f**。
伪版本由 Go 远程解析生成，非手工编造。模块路径保持 botgo-plus，最低 Go **1.18**。
SDK 分支后续 `21dfd3b` 仅固定 examples 依赖并补充文档，未改变已验证的运行时代码；
本框架固定上面的实现提交，不跟踪分支或 main，没有创建正式 Tag。

| 校验信息 | 值 |
| --- | --- |
| 模块 Sum | `h1:Dd/cV+OUKUlLgwrJ1MEBOIExhI40C+e4e15ayHrRgEs=` |
| go.mod Sum | `h1:eCOAAdQZLKG0VRKdCje1x/bx41a+y9W9yT3FgDlkMVs=` |

## SDK 审查、保留能力与同步

[SDK 完整交付说明](https://github.com/kylin930/botgo-plus/blob/feat/shizukubot-protocol-sync/docs/shizukubot-protocol-sync.md)
包含完整目录差异、公开 API AST 清单、来源提交和测试边界。
审查比较了 vendored P2.3、P2.2→P2.3 SDK diff、botgo-plus main 和官方参考 BotGo
`fe31c0d`，按差异块合并，没有将源 SDK 整体覆盖目标。

- 既有 1,222 个公开符号/字段无删除或类型签名不兼容变化。增加 IsYou/MemberRole/Scope、普通群消息相关类型/handler/Intent、FileInfoRaw、PlatformError、请求专属 TraceID 等。
- 保留 MemberOpenID/UserOpenID、Union 字段、状态和管理/订阅/好友事件、ENTER_AIO、扩展 Intent、媒体 UUID/TTL、URL/base64、Stream/PromptKeyboard/ActionButton/FeatureID 等既有扩展。
- 两项有意的线协议纠正：msg_type=0 与 srv_send_msg=false 不再被 omitempty 省略，原属性名和 Go 类型不变。
- 迁移 opaque file_info、新旧时间戳、HTTP/业务错误、并发授权/TraceID、Token 失效后的下一次调用恢复、签名字段校验。不自动重试非幂等发送。
- 独立 SDK 额外验证发送返回消息 ID、上传返回 file_info，支持成功响应的 Message.TraceID，保持旧 errs.Error 和 TraceID() 调用兼容；后者仍是最后完成请求。
- SDK 独立 Webhook API 保留并修复 chunked body/超大请求/缺失字段/错误请求和日志。ShizukuBot 继续使用现有按 Bot 接收的 Webhook adapter，不安装 SDK 全局处理器。
- 既有 Go 1.16 声明与 oauth2/x/net 已有依赖的 Go 1.18 要求不一致，修正为 1.18，并用 1.18.10 实测，不跟随框架升到 1.26。

SDK 的普通单测、本地 HTTP、race、vet、build 通过；Go 1.18.10 测试/构建通过。
历史 Redis 测试明确 Skip，真实 QQ API examples 被 build tag 与环境变量双重隔离；
二者均未执行，不作为通过项。旧 SimulateSearch 测试改为本地签名 Mock，不再访问公共 QQ 网页。
SDK examples 子模块原 v0.1.7 不存在，已固定到上述实际伪版本，并独立 tidy/test/vet/build 通过。

## 框架迁移与本地目录删除

F1 仅将 11 个活跃 Go 文件中的导入前缀替换为 botgo-plus，覆盖：

- core Bot、events、reply、diagnostics、安全日志及测试。
- transport/qq Gateway 及测试。
- webhook handler 及测试。

移除 `github.com/tencent-connect/botgo v0.14.0` 和原 SDK replace，tidy go.sum。
没有把两套 module 的 dto/openapi 类型混用；oauth2.TokenSource 的路径和类型不变。
主程序、诊断 CLI、插件和规则均通过原有框架 API 间接使用同一发送能力。
没有修改插件 SDK、EventBus、Dispatcher、规则匹配逻辑、回复上下文/序号分配、多 Bot 或 WebUI。

先在独立临时消费者验证远程模块下载、校验、编译和 SDK 全部普通测试；再让
ShizukuBot 使用外部依赖运行全量测试通过，最后执行 `git rm -r botgo-master`，
删除 **182 个跟踪文件**。最终目录不存在，未转成 submodule/vendor/legacy 或其他目录复制。
旧目录仅保留于 Git 历史，重要变更说明已整理进 SDK 文档。

P0/P1/P2.3 历史报告和协议核对记录仅增加“历史实现”说明，原来的依赖与验证事实
保持原文；当前 README/架构说明指向独立 SDK。

## 远程下载与删除后的干净验证

两次独立远程验证均不使用本地 SDK 路径：

1. SDK 推送后，空 GOMODCACHE 的临时消费者解析提交为实际伪版本，下载、`go mod verify`、构建，并从下载目录运行 SDK 普通测试。
2. 删除本地目录后，ShizukuBot 使用另一个全新 GOMODCACHE 和 GOCACHE，从仓库根目录重新 tidy/verify/list/test/build。Go 的 module Dir 指向该缓存中的 botgo-plus，不是 `/workspace/botgo-plus`。

checksum/sumdb 校验保持启用。只读 home 下额外设置可写 GOPATH，用于 sumdb checkpoint；
没有使用 GOSUMDB=off、临时 SDK replace 或手工复制模块源码。

复现命令（用自己新建的空临时目录替换以下路径）：

```sh
export GOPATH=/tmp/shizuku-migration-gopath
export GOMODCACHE=/tmp/shizuku-migration-clean-cache
export GOCACHE=/tmp/shizuku-migration-build-cache
test ! -d botgo-master
go mod tidy
go mod verify
go list -m all
go list -m -json github.com/kylin930/botgo-plus
go test ./...
go build ./...
```

其余验证包括 `go test -race ./...`、`go vet ./...` 和
`CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`。

## P2.3 与现有回归

保留全部框架 QQ Mock/签名/插件/规则测试，仅修改它们的 SDK import。
验证内容包括 C2C 与群 /ping、内置命令、规则保存即生效、插件/规则共同回复、
每事件 msg_id/msg_seq 隔离/并发/上限、跨 Bot 目标隔离、event_id 支持边界、
所有现有媒体格式、opaque file_info、平台非 2xx/200 业务失败、超时/取消/不重放、
Webhook 原文签名/挑战/ACK/停止后处理/重复去重、Gateway 重连/心跳和 SDK 类型一致性。

框架普通测试、race、vet、Linux 构建和 Windows amd64 交叉构建通过。
P0 至 P2.3 的插件管理、沙盒和规则现有测试保留。
独立协议回归可运行：

```sh
go test ./core ./webhook ./transport/qq -run 'TestQQ|TestWebhook|TestSigned|TestGateway'
```

## 残留检查

活跃 Go 源码、go.mod/go.sum、构建工具没有旧官方 SDK 导入或本地 SDK 引用。
`go list -m all` 仅列出固定 botgo-plus，不含旧 BotGo 或替换。
全仓搜索的旧名称只出现在明确标记的历史报告，以及解释此次迁移/回滚的文档。
生成插件与旧数据迁移测试中的 `replace shizukubot => 临时工作区` 是既有框架测试
fixture，不是 SDK replace，不被当作 SDK 迁移残留删除。

## 尚未完成的真实联调与合并顺序

**真实 QQ 联调未执行**：当前环境没有自有测试机器人的凭证、成员/群授权或官方
HTTPS 回调配置。上述 /ping、签名、媒体等结果是本地 Mock；不代表 QQ 客户端实收。
真实验证继续使用 [QQ 接入指南](qq-integration-guide.md) 的 B1–B6 流程。

建议先审查合并 botgo-plus SDK 分支，再合并框架迁移分支；本轮不自动合并。
固定提交本身已可远程下载，但正式 Tag/Release 需另行决定。

## 回滚

优先部署/检出迁移前已验证的 ShizukuBot 提交 `8f396398`（在单独部署目录操作），
或在当前分支创建恢复原依赖的 Git revert：先撤销本地 SDK 删除提交，再撤销导入/
Go Modules 迁移提交，之后重新执行测试与构建。不要从网络或其他电脑复制未知 SDK 文件。
SDK 分支保持独立，框架回滚不要求改动 SDK main，不使用强制推送或重置共享分支。

## HTTP / MDUI 与提交记录

迁移后二进制执行 HTTP 冒烟通过：实际启动/登录/API/静态资源、100 次并发全部成功、
凭证脱敏和优雅退出。真实 Chromium + MDUI 2 的三个桌面/移动端脚本均通过：
插件配置与实例/沙盒、规则编辑与即时应用、QQ 诊断未验证状态与安全展示。
没有修改页面实现，截图仍是本地假 Bot 数据，不是 QQ 平台验收证据。

```sh
go build -o /tmp/shizukubot-p231 .
go build -o /tmp/shizuku-http-p231 ./cmd/concurrency-test
python3 scripts/p0-smoke.py --binary /tmp/shizukubot-p231 --runner /tmp/shizuku-http-p231
python3 scripts/p2-browser-smoke.py --binary /tmp/shizukubot-p231
python3 scripts/p22-browser-smoke.py --binary /tmp/shizukubot-p231
python3 scripts/p23-browser-smoke.py --binary /tmp/shizukubot-p231
```

本轮新截图分别留于本机 `/tmp/shizuku-p231-{plugins,rules,diagnostics}-browser`。
Chromium 使用环境已配置的受信任 CA，执行时需正常访问用户 NSS 数据库；
未通过忽略证书错误或伪造 MDUI 资源来通过测试。

框架阶段提交：F1 `96885f6`（固定模块/import），F2 `f1d1bbb`（删除旧 SDK），
F4 单独提交当前文档/历史标记。SDK 的差异审查、实现、测试、示例固定亦分别提交。
SDK 公开 DTO 为新增字段，外部调用方如使用非命名 struct literal 可能需要改用
命名字段；既有命名字段、类型和 JSON 属性名保留，框架现有调用无需调整。
