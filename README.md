# ShizukuBot v2

Go 原生 QQ 官方机器人框架，提供多机器人、消息和富媒体、MDUI 2 管理后台、日志统计、配置、认证与调试沙盒。后端和插件统一使用 Go；LiteGraph、旧图引擎和脚本插件运行时已移除。

## 构建与运行

需要 Go **1.26+**（本轮验证 1.26.10）。后端与代码插件仅需 Go 工具链，不需要 Node.js、npm 或前端构建步骤。已有生成的注册表随源码提交。

```sh
go mod download
go build -o shizukubot .
./shizukubot
```

Windows 使用 `go build -o shizukubot.exe .`。从仓库根目录运行，保留 `webui/` 静态文件；WebUI 初始端口 4545。首次管理员密码写入本机 `data/.admin-initial-password`，不输出到日志；登录后请修改密码。QQ 凭证填写在本机配置中。HTTPS 反向代理部署设置 `webui.secure_cookies: true`，保留正确 Host；安全状态不直接取自代理协议头。

## 统计与 WebUI 外观

收发计数、用户/群去重、DAU、小时和每日历史保存在 `data/stats.db`，跨重启恢复。
默认保留最近 90 天，每天 Asia/Shanghai 03:00 清理；可在配置页修改保留期和清理时间，立即应用。
SQLite 驱动无需 CGO，Linux/Windows 均只需 Go 工具链。备份需同时保存数据库和 `data/.stats-hmac-key`。

配置页支持默认背景、图片 URL、本地图片上传，以及 MDUI 动态主色和浅色/深色/跟随系统。
预览可取消，保存后服务器持久化，登录页与管理后台共享外观。
[WebUI 使用与配置说明](docs/webui-guide.md) · [统计存储与口径](docs/statistics-storage.md) · [P2.5 交付报告](docs/v2-p25-stats-appearance-report.md)

## QQ 官方接入与联调

QQ SDK 统一维护于 [botgo-plus](https://github.com/kylin930/botgo-plus)，通过 Go Modules
固定为 `v0.0.0-20261102144408-979b74884c88`，无需复制 SDK 源码或设置本地 replace。
[SDK 迁移报告](docs/v2-p23-botgo-plus-migration-report.md) 记录版本、干净构建和回滚方式。

新 Bot 默认使用签名 Webhook：`/webhook/{botID}`；显式 WebSocket 配置继续兼容。
插件 SDK、内置命令和规则共用事件级被动回复上下文，携带原始 `msg_id`，并发安全
分配 `msg_seq`（当前官网：C2C 最多 4 次、群聊 5 次；失败/超时也计入尝试）。
主动管理 API 不复用入站消息 ID，平台权限/限频错误会明确返回。

Bot 管理页可展开 QQ 联调诊断；没有事件/回复证据时显示“暂无记录”或“未检测”。
先用 `go run ./cmd/qq-diagnose --bot 你的BotID` 离线检查，再按
[真实测试机器人接入指南](docs/qq-integration-guide.md) 配置 HTTPS、测试成员和权限。
**本次没有真实 QQ 凭证，真实 Token/官方回调/客户端收信尚未验证。**
Mock 测试通过不等于真实平台验收通过；协议版本差异见 [审查记录](docs/qq-protocol-audit.md)。

## Go 插件

插件是普通 Go package，公共 API 在 `shizukubot/sdk`。最小 Ping 无需结构体、工厂或空生命周期：

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

监听消息可用 EventPlugin；多个指令/事件可用 Define。需要资源初始化、后台任务和清理时保留完整 Extension 接口，详见 [五分钟创建插件](docs/plugin-development.md)。Simple 与 Advanced 共用同一管理器。

| 包 | 功能 |
| --- | --- |
| `plugins/hello` | CommandPlugin：`/ping` → `pong` |
| `plugins/autoreply` | EventPlugin：按 Bot 配置关键词和回复 |
| `plugins/example` | Advanced：生命周期、配置与持久计数 |

另保留 `plugins/framework` 的 `/framework` 指令。默认不启用；在 WebUI 从真实 Bot 下拉列表选择机器人，用 Schema 表单保存配置后启用。配置、启停、重启和沙盒操作无需重新编译。

新增插件在 `plugins/<name>/plugin.go` 导出 `Definition() sdk.Definition`。Simple 构造器自动创建新实例；Advanced 工厂也必须为每个 Bot 返回新对象。源码变更须审核、生成注册表、测试、构建并重新部署：

```sh
go generate ./pluginregistry
go test ./sdk ./plugins/... ./pluginregistry
go build -o shizukubot .
```

WebUI 只管理已编译插件，提供搜索筛选、独立状态、Schema 表单及 JSON 编辑、实例启停、配置/状态备份和结构化沙盒。保存可应用到当前插件实例；不上传源码安装运行。用法见 [插件管理与沙盒](docs/webui-plugin-management.md)。插件运行于主进程，必须可信并遵守 context；调试沙盒捕获回复、隔离临时状态，不提供文件/网络安全隔离。

## 无需编程的自动回复

进入 WebUI“规则”（`/#/rules`），选择真实 Bot，新建“包含你好 → 回复你好！”规则。新建默认禁用，可先测试草稿，再保存并启用；编辑、启停、删除立即作用于后续事件，不需要编译或重启。支持完全匹配、包含、前缀、群/C2C 范围、优先级与安全文本占位符。

规则使用现有 EventBus、去重及回复能力。内置命令优先；规则和 Go 插件可同时响应，同关键词的 autoreply 插件可能额外回复。规则测试仅捕获预期结果，不发送 QQ，也不调用生产插件。操作、限额和版本冲突见 [规则指南](docs/rules-guide.md)，实现、测试与真实截图见 [P2.2 报告](docs/v2-p2-rules-report.md)。

## 旧数据升级

先停止旧程序并备份数据：

```sh
./shizukubot migrate-legacy --dry-run
./shizukubot migrate-legacy --apply
# 默认生成目录 plugins/migrated；检查备份、report.json 和生成源码后再注册构建。
go generate ./pluginregistry
go build -o shizukubot .
```

原始 JSON 不删除。只有可证明语义的完整线性回复链生成 Go 包；复杂图保留备份并报告人工迁移。生成源码不等于已安装、启用或完成部署。旧脚本目录不再加载，请离线保存原项目并人工改写为 Go；旧格式市场安装入口保持停用。

## 验证与文档

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./...
# 独立 QQ HTTP 请求协议回归，不需要凭证：
go test ./core ./webhook ./transport/qq -run 'TestQQ|TestWebhook|TestSigned|TestGateway'
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o shizukubot.exe .
```

- [P2.3.1 SDK 统一维护与依赖迁移](docs/v2-p23-botgo-plus-migration-report.md)
- [P2.3 QQ 联调交付报告](docs/v2-p2-qq-integration-report.md)
- [架构](docs/architecture-v2.md)
- [插件开发](docs/plugin-development.md) / [Go SDK](docs/plugin-sdk.md)
- [旧数据迁移与回滚](docs/legacy-migration.md)
- [P2.2 规则与自动回复交付报告](docs/v2-p2-rules-report.md)
- [P2.1 插件后台与沙盒交付报告](docs/v2-p2-plugin-ui-report.md)
- [P1.2 SDK 开发体验与测试报告](docs/v2-p1-sdk-dx-report.md)
- [P1 Go 纠偏历史报告](docs/v2-p1-go-correction.md)
