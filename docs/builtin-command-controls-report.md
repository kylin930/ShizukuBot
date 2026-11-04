# 内置指令开关与 Tooltip 调整

2026-11-04，基线 main `57acf86`，已包含 P2.5 / PR #7。
分支 `feat/builtin-command-controls-tooltips`，未合并 main。

## 指令开关

配置页“内置指令”新增三个全局选项，保存后生效并持久化：

| 配置字段 | 用途 | 旧配置默认 |
| --- | --- | --- |
| `builtin_commands.help_enabled` | 启用 `/help` | true |
| `builtin_commands.status_enabled` | 启用 `/status` | true |
| `builtin_commands.show_shizuku_in_help` | 在 `{指令列表}` 中显示 `#shizuku` | true |

`#shizuku` 始终可执行，没有禁用开关。隐藏选项仅控制自动帮助列表，自定义模板原文保留。
`/help`、`/status` 使用 Bot 的指令前缀；禁用后核心不再保留对应名称，规则和插件可接管。
帮助列表过滤禁用的内置项并去重，保留插件提供的可用指令。

配置 API 返回明确布尔值；旧客户端只编辑模板、未发送新字段时保留已保存的开关。
继续使用配置版本校验、原子保存、脏表单保护；运行时模板/开关以独立快照发布。

## 提示与文案

规则说明、插件重启说明、社区说明、沙盒范围/测试说明和 QQ 接入帮助统一为 MDUI 2 Rich Tooltip。
支持悬停、键盘聚焦与 Esc、手机真实触摸点按及点外关闭；长说明可滚动，减少动态效果时关闭 Tooltip 的进入/退出动效。
QQ 诊断、错误原文、Manifest/Schema 和备份操作属于功能内容，保留可查看/可操作的入口。
沙盒长链接改为滚动查看完整原文，取消“展开完整内容”。

界面内置文案及 API 自有错误提示中的 `．`、`。` 已改写或去掉句点。
必要的认证、冲突、删除和备份限制保留；消息、日志、模板及后台返回的原始诊断数据不做替换，URL、版本和文件名中的 ASCII 点保留。

真实浏览器发现 MDUI 原生 Tooltip 在展开过程中卸载会留下异步动画/滚动回调。
旧内容先隐藏并设为 inert，等原生 closed 事件和组件更新结束，在帧内回调完成后的任务边界移除；新内容立即渲染。
插件/规则局部详情共用此回收方法，旧节点移除选择标识，避免重复 ID 干扰焦点和定位；不修改组件内部实现，也不延迟新页面请求。

## 验证

- Go 全量 test、race、vet、build 通过
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...` 通过
- 核心/API 测试覆盖默认兼容、独立禁用、固定 `#shizuku`、帮助过滤、指令前缀、保存/读取、旧客户端保留字段、配置冲突与规则接管
- 新专项 Chromium 验证开关保存/刷新、键盘/Esc/悬停、真实触摸、Tooltip 关闭与页面/详情切换、草稿保护、360px 布局、reduced-motion 和文本注入防护，无未处理异常
- 原 P2、P2.2、P2.3、P2.4、P2.5 浏览器回归通过，包含 P2.5 的 44 组布局检查
- P0 HTTP 冒烟通过，100/100 并发请求与优雅关闭正常

截图来自隔离运行的 WebUI，不连接 QQ：

- [桌面指令设置与 Tooltip](images/builtin-controls/builtin-controls-desktop.png)
- [手机触摸 Tooltip](images/builtin-controls/builtin-controls-mobile.png)
- [浏览器验证记录](images/builtin-controls/verification.json)

复现专项：

```sh
go build -o /tmp/shizukubot-controls .
python3 scripts/builtin-controls-browser-smoke.py --binary /tmp/shizukubot-controls --artifacts /tmp/builtin-controls-screenshots
```
