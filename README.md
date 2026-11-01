# ShizukuBot v2

Go QQ 官方机器人框架，保留 MDUI 2 管理后台、多机器人、日志统计、消息与富媒体、用户认证和调试沙盒。P1 使用代码插件；生产运行时已移除 LiteGraph、节点引擎和 Yaegi。

## 运行

需要 Go **1.26+**（本轮使用 1.26.10）。JavaScript 插件需要 **Node.js 24 LTS**（`.node-version` 固定开发版本 24.19.0）；Go 核心无需 Node 即可编译，启用 JS 插件前必须配置 Node 到 PATH。

```sh
go mod download
go build -o shizukubot .
./shizukubot
```

Windows 使用 `go build -o shizukubot.exe .` 和 `shizukubot.exe`。从仓库根目录运行，WebUI 位于配置的端口（初始 4545）。首次管理员密码写入本机 `data/.admin-initial-password`，不输出到日志；登录后请修改密码。QQ 凭证在本机配置中填写，不进入插件上下文。HTTPS 反向代理部署请设置 `webui.secure_cookies: true`，保留正确 Host；不信任代理协议头。

## 插件

三个可运行示例在 `examples/plugins/hello`、`keywords`、`counter`；预编译 JS 已入库。SDK 在 `sdk/`，Go 静态扩展注册表在 `plugins/builtins.go`。

```sh
npm ci --ignore-scripts
npm run build:examples
# 在示例目录中将 manifest.json、package.json、dist/ 和可选 src/ 打成 ZIP，根层不要增加目录。
```

在插件页面安装 ZIP，填写实际 Bot ID 或独立调试标识，检查权限和配置，审核后确认信任，再启用或进行沙盒测试。默认不运行未信任社区代码。插件的重启按钮会重载本地代码和配置，不需要重启核心。

独立 Node 进程是故障隔离边界，**不是强安全沙盒**；可信插件仍可能直接读写文件或访问网络。

## 旧版本升级

先停止旧程序，保留 `data/plugins` 和 `data/custom_nodes`：

```sh
./shizukubot migrate-legacy --dry-run
./shizukubot migrate-legacy --apply
# 检查输出的备份与迁移报告。生成项目默认禁用，不会自动确认信任。
./shizukubot migrate-legacy --rollback /path/to/backup --destination data/code-plugins
```

原 JSON 不删除。简单、完全可核对的线性回复才自动转换；复杂分支、代码和自定义节点必须人工迁移。旧格式市场安装入口已停用。

## 验证与文档

```sh
go test -race ./...
npm run build:examples
node --check webui/static/js/app.js
```

- [架构](docs/architecture-v2.md)
- [插件开发](docs/plugin-development.md) / [SDK 与 IPC](docs/plugin-sdk.md)
- [迁移与回滚](docs/legacy-migration.md)
- [P1 交付、测试与限制](docs/v2-p1-report.md)
