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

## Go 插件

插件是普通 Go package。SDK 位于 `plugins/`，三个示例是：

| 包 | 功能 |
| --- | --- |
| `plugins/hello` | `/ping` → `pong` |
| `plugins/autoreply` | 按 Bot 配置关键词与回复，监听群/C2C 消息 |
| `plugins/example` | `/count`，独立配置与持久计数 |

另保留 `plugins/framework` 的 `/framework` 指令。默认不启用；在 WebUI 选择 Bot，保存配置后启用。配置、启停、重启和沙盒操作无需重新编译。

新增插件在 `plugins/<name>/plugin.go` 导出 `Definition() plugins.Definition`，构造函数为每个 Bot 返回新对象。源码变更须审核、生成注册表、测试、构建并重新部署：

```sh
go generate ./pluginregistry
go test ./plugins/... ./pluginregistry
go build -o shizukubot .
```

WebUI 只管理已编译插件，支持配置/状态数据备份与恢复；不上传源码安装运行。插件运行于主进程，必须可信并遵守 context；调试沙盒捕获回复、隔离临时状态，不提供文件/网络安全隔离。

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
go test -race ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o shizukubot.exe .
```

- [架构](docs/architecture-v2.md)
- [插件开发](docs/plugin-development.md) / [Go SDK](docs/plugin-sdk.md)
- [旧数据迁移与回滚](docs/legacy-migration.md)
- [P1 Go 纠偏与实际验证结果](docs/v2-p1-go-correction.md)
