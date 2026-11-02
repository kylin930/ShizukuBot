# QQ 官方测试机器人联调指南

本指南面向自己创建、已授权的 QQ 官方测试机器人与测试账号/群。
本次交付没有真实凭证，**真实联调未执行**；Mock、沙盒、Token 成功均不能证明 QQ 客户端收到回复。

## 平台准备与本机配置

1. 在 [QQ 开放平台](https://q.qq.com/) 创建/选择自己管理的机器人，按当前控制台要求完成主体认证和开发设置。配置沙箱/测试成员，只让自己授权的账号和群参与测试；部分事件、群聊、Markdown、按钮等能力需要独立权限或审核。
2. 在平台获取 AppID、AppSecret，在本地管理后台填写。AppSecret 为只写字段，编辑时留空保留原值。不要发到 PR、Issue、聊天或命令行参数，不要写进 URL 查询参数。
3. 本地 `config/config.json`（已 gitignore）示例结构如下，**占位符不是凭证**。在受信任的本地编辑器填写，不通过 shell 拼接密钥：

```json
{
  "webui": {"enabled": true, "host": "127.0.0.1", "port": 4545, "password": "本地随机会话签名密钥"},
  "bots": [{
    "id": "qq-test", "name": "QQ 测试", "app_id": "你的 AppID",
    "app_secret": "你的 AppSecret", "enabled": false, "mode": "webhook",
    "plugins": ["hello"], "command_prefix": "/", "admin_users": []
  }]
}
```

4. Unix 上使用 `chmod 600 config/config.json`，配置目录建议 700；Windows 上限制文件 ACL 为运行账号。框架保存配置会设置 0600。沿用本地文件兼容性，本次没有增加环境变量凭证插值。不要把含凭证的文件复制到仓库其他位置。`.env*`、本地 credentials、token-cache、联调 artifacts 和日志也已排除版本控制。
5. 从仓库根目录运行 `go run .` 或 `go build -o shizukubot .` 后运行程序。管理后台默认仅本机监听；首次管理员密码在 `data/.admin-initial-password`。不要将整个管理后台公开。
6. 用已有反向代理提供合法公网 HTTPS 域名和证书，只转发 `POST /webhook/qq-test` 到本机端口；不要转发 `/api/`、`/ws/` 或登录页。例：Nginx `location = /webhook/qq-test { proxy_pass http://127.0.0.1:4545; proxy_set_header Host $host; }`。不依赖特定云服务。代理需保留原始请求体及 `X-Signature-Ed25519`、`X-Signature-Timestamp`，不要改写 JSON。
7. 在平台设置 HTTPS 回调地址 `https://你的域名/webhook/qq-test`，完成回调地址验证并订阅允许的 C2C_MESSAGE_CREATE、GROUP_AT_MESSAGE_CREATE；GROUP_MESSAGE_CREATE 仅在实际授权支持时使用。校验和心跳不要求 Bot 已启动；普通消息必须先启动 Bot。平台端口支持情况以当前控制台为准。
8. 按平台要求设置 OpenAPI 出站 IP 白名单。填写真正运行机器的公网出口 IP，而非回调域名的入站 IP；NAT/代理可能改变出口。403 不能单独证明一定是白名单错误。
9. 在插件页为该 Bot 启用 `hello`，在 Bot 管理页启动。展开“QQ 联调诊断”：运行状态、接收事件时间/类型/是否有消息 ID、最近被动回复接受时间和错误分类分别检查。Token 认证成功不代表回调正常。
10. 在 QQ 中用授权测试成员发消息。观察事件到达、OpenAPI 接受，再人工确认客户端看到回复。刷新 Bot 列表更新诊断；诊断为内存最新状态，重启后重置，不保存聊天内容。

## 默认离线诊断与显式 live 认证

```sh
go run ./cmd/qq-diagnose --config config/config.json --bot qq-test
# 只在自己已授权的测试 Bot 上显式执行，不发送任何消息：
go run ./cmd/qq-diagnose --config config/config.json --bot qq-test --live-auth
```

默认只读配置，不保存文件、不访问 QQ、不自动启动 Bot。工具永远不发送消息，
没有自动操控客户端的能力。退出码：0 本次检查通过；2 参数缺失；3 文件/JSON
错误；4 Bot/凭证/模式不完整；5 显式 live Token 认证失败。缺少条件会说明真实验证未执行。
不输出 AppID/Secret/Token/原始响应。live 认证只验证 Token 获取，后续消息需要下面的人工流程。

## 人工验收清单（请填写真实证据）

用私有本地验收记录保存时间、步骤、结果、脱敏平台码/trace；不记录完整私人消息、凭证。
状态使用“通过 / 失败 / 未执行 / 未具备测试条件”，不要把 Mock 结果填入。

| 编号 | 操作与预期 | 本次真实结果 |
| --- | --- | --- |
| B1-a | 离线诊断后显式 live-auth；正确凭证能认证 | 未执行：无凭证 |
| B1-b | 本地测试配置中临时使用错误凭证，认证明确失败，再恢复 | 未执行 |
| B1-c | 模拟本机网络不可达后恢复，Token 获取失败不泄露凭证 | 未执行 |
| B1-d | 等待真实 Token 到期再发送新的测试消息；应懒刷新；若平台提前失效，首次请求明确失败，下次调用重新取 Token，不自动重放旧请求 | 未执行 |
| B2 | QQ C2C 发 `/ping`：事件类型正确、有 MessageID、hello 匹配、平台接受、QQ 客户端实际收到 `pong` | 未执行 |
| B3-a | 规则页创建已启用规则：contains `你好` → reply_text `你好！`，来源 C2C/群；保存后立即发消息确认 | 未执行 |
| B3-b | 修改回复文本、禁用/再启用，均无需重连 QQ；重启进程后规则恢复 | 未执行 |
| B4 | 自己的官方测试群按支持方式 @Bot `/ping`，回复只进原群；再测第二授权群与 C2C 的隔离 | 未具备测试条件：无群聊权限信息 |
| B5 | 一条 `/ping` 用 hello + 匹配该关键词的规则产生 2 次回复；seq 应分别为 1/2；C2C 总尝试不超过 4，群聊不超过 5 | 未执行 |
| B6-a | 临时网络断开/平台超时：明确失败或送达未知，不自动重发；恢复后发新消息可正常回复 | 未执行 |
| B6-b | 暂停 Webhook 服务后恢复；平台可能重投，进程内同 ID 在去重窗口内只执行一次 | 未执行 |
| B6-c | 停止 Bot 后消息不可执行，启动后新消息正常；无效配置不误报运行成功 | 未执行 |
| B6-d | 重启框架、检查规则/插件配置持久化并重新完成 `/ping` | 未执行 |

B5 只需两次回复即可证明链路，无需主动压测限频。内置 `/help`、`/status`、
`#shizuku` 也使用同一事件 Reply；原有保留命令策略不变，插件/规则不抢占内置命令。

## 平台能力与诊断边界

- [协议核对和版本差异](qq-protocol-audit.md)：当前官网 C2C 上限 4、群聊 5；旧 GitHub 文档写 C2C 5，不采用旧值。C2C 时效介绍 60 分钟、字段说明 5 分钟存在冲突，因此收到即回复，不实现延时复用。
- 当前官网列出主动发送权限/频控。管理 API `/api/bots/{id}/message/{c2c|group}` 是**明确主动发送**，需要运行 Bot、指定已获授权目标；没有 msg_id，不借用上次消息、不伪造事件。普通回复必须走 Event.Reply，不能改调 SendGroupMessage 来绕过次数限制。平台拒绝则返回脱敏错误码，绝不转换成伪被动回复。
- 图片/视频先上传且 `srv_send_msg:false`，再用当前上下文发送。URL 回源、base64 上传、Markdown/keyboard 的真实权限仍需平台验证。未新增分片上传、召回消息、互动事件回复。
- Webhook 所有请求保留签名验证。op12 d=0 表示事件已接收/去重，不代表业务回复成功；停止/未接收返回 d=1。接收处理有 10 秒协作超时；插件须遵守 context，不能强制杀死不合作的可信 Go 扩展。
- `INTERACTION_CREATE` 被规范化/接收，但本轮不推断目标，不提供普通消息 Reply；不能把 interaction ID 作为 msg_id。
- WebSocket 保留 Discover/Identify/Heartbeat/Reconnect/Resume/Seq/Shard/取消代码与 Mock 回归。旧 GitHub 停用公告与当前官网保留文档不一致，不能据此断言你的 Bot 获得接入资格。Webhook 是新配置默认值，显式 websocket 旧配置仍生效；本轮无真实 Gateway 验证。
- 错误类别：认证、网络/协议、送达未知的超时、缺少关联 ID、次数限制、过期、主动发送无权限、群成员/消息类型权限、平台限频、未知错误。HTTP 403 仅标为权限或 IP 策略；不把未知码硬判成白名单错误。
- OpenAPI 成功仅表示平台接受，不等同端到端送达。沙盒和规则预览为捕获模式，不会更新 QQ 成功诊断。
