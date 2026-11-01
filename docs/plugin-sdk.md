# Plugin SDK / IPC v1

TypeScript 的完整声明在 `sdk/index.d.ts`，Go 模型/接口在 `plugins/model.go`。

## JS 插件导出

`definePlugin({id,name,version,commands?,events?,load?,start?,stop?,unload?})` 是声明辅助函数，不绕过主机验证。commands 的 key 不带 `/`；events 的 key 为规范事件名或 `*`。多个插件注册同名普通指令会按订阅顺序分别执行，互不覆盖。生产环境核心内置命令优先，已由核心处理的命令不会再次交给插件；独立沙盒不运行核心内置命令。一个插件里同时注册指令和消息事件时两者均可执行。

指令以 `/` 开头，支持 Unicode 空白、单双引号、反斜杠转义；未闭合引号视为无效命令，不进行 shell 展开。原消息始终保留在 ctx.message。已知机器人 mention 会在命令解析前移除，保留其他用户 mention；缺少元数据时使用 QQ mention 语法回退。Bot 的内置指令仍使用 Bot 配置的前缀，SDK 指令目前统一 `/`，其他前缀可由事件监听显式处理。

## Context

| 字段/方法 | 含义与权限 |
| --- | --- |
| botId / userId / groupId / channelId / guildId | 当前事件身份，不包含核心对象 |
| eventType / platformType / eventId / messageId / source | 规范事件类型、平台原始类型与身份 |
| data | 平台事件 d 对象；不可信输入，可能为 null；不是完整核心配置或鉴权头 |
| botMentionIds / message / args | 原消息、bot mention 身份及解析后的参数 |
| config | config.read；独立快照，无权限时为空对象 |
| reply(text 或 Reply) | message.send；当前事件的回复能力 |
| replyImage(url)、replyRichMedia(text,url) | 图片或富媒体 |
| replyMarkdown(text)、replyKeyboard(markdown,keyboardJSON) | Markdown / 按钮 |
| replyVideo(url) | 视频 |
| state.get<T>(key)、state.set(key,value) | state.read / state.write；按插件和 Bot 隔离 |
| log(message) | log.write；写入管理日志，最多 4096 字符 |

Reply 的 kind 为 text、image、rich_media、markdown、keyboard、video，可带 text/url/keyboard。群/C2C 继续使用核心已有媒体发送实现，支持原有 URL 与 data:image 路径；频道/私信当前提供文本回复，其它 kind 返回明确的 unsupported 错误。真实 QQ 媒体调用仍需平台授权，本轮使用 Mock/捕获能力验证。

load 收到 `{botId,config}`，start/stop/unload 不含事件发送能力；事件处理中必须 await 能力调用。SDK 不提供任意 Bot sender，也不暴露 AppSecret、TokenSource、管理 Cookie/密码或父进程环境。

## Go

Extension：Load(context,Host)、Start(context)、HandleEvent(context,Call)、Stop(context)、Unload(context)。Host 提供 Context()、Command(name,handler)、On(type,handler)。Call 包含 Event、Args、Config、State、Log。工厂通过 Manager.RegisterGo 注册，启动、禁用和卸载由 Manager 串行管理。

初始化 context 与长期生命周期分开：后台任务用 Host.Context()。同实例事件串行，不同实例可并行；调用不得保存并在事件结束后使用 Reply。原生 Go 崩溃/不合作代码的强隔离不在 SDK 承诺之内。

## IPC

每行 JSON，UTF-8，`v:1`；未知版本、错误 ID、错误 parent、无效 JSON 或超限帧使 Worker 失败。

```json
{"v":1,"id":"7","type":"event","payload":{"command":"ping","args":[],"event":{"botId":"configured-bot","eventType":"c2c_message","message":"/ping"}}}
{"v":1,"id":"cap-1","parent":"7","type":"reply","payload":{"kind":"text","text":"Pong!"}}
{"v":1,"id":"cap-1","parent":"7","type":"capability_result","payload":null}
{"v":1,"id":"7","type":"result","payload":{"handled":1}}
```

主机请求：load、event、stop。Worker 能力请求：reply、state.get、state.set、log。返回 error 字符串表示失败，不能当成功继续。load 返回 commands/events/version，注册表和权限由 Go 验证。stdout 专用于 IPC；console 输出进入受限 stderr 日志，插件不可依赖 stdout 打印业务数据。

默认每次 JS 事件执行 2 秒；超时/取消会终止 Node Worker（正在执行的主机能力依赖 SDK 协作取消，TokenSource 认证接口自身无 context），后续匹配事件触发有退避的重建。每个 Worker 的 JS old-space 上限 96MiB，不等于完整进程 RSS 或系统资源限制。禁止依赖启动后未 await 的后台能力请求；后台自行联网也不受 IPC 权限约束，只有经审核的可信代码才可运行。

## 管理 HTTP

所有接口沿用管理员认证与 P0 CSRF 策略：

- GET `/api/plugins`，GET `/api/plugins/{id}?bot_id=...`：列表、manifest、状态与指定 Bot 配置。
- POST `/api/plugins`：`{package_base64,restore_data?:false}`，安装代码包或显式恢复备份。
- PUT `/api/plugins/{id}`：`{bot_id,config?,trusted?}`，更新配置或信任；撤销信任同时禁用。
- POST `/api/plugins/{id}/toggle`：`{bot_id,enabled,trust?:false}`。
- POST `/api/plugins/{id}/restart`：`{bot_id}`，显式重载。
- GET `/api/plugins/{id}/export` 导出代码；`?backup=1` 包含私有配置与状态。
- DELETE `/api/plugins/{id}`：停止所有实例、撤销信任、卸载代码；保留配置和状态。
- POST `/api/bots/{id}/sandbox/send`：`{type:"group"|"c2c",content,plugin_id?}`；返回 replies、results、errors、executions。

沙盒不会进入生产订阅或复用生产状态。未信任插件不能以“调试”为理由绕过信任。旧 graph/custom-nodes/code2graph/graph2code API 和旧静态资源路由已不存在。
