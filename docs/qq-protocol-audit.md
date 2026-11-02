# QQ protocol audit (2026-11-02)

Baseline: `fc4db97` (remote main, merged PR #3 / P2.2). Dependency:
`github.com/tencent-connect/botgo v0.14.0 => ./botgo-master`. This replacement
contains Gensokyo-derived DTO/event/base64 upload patches (SDK_CHANGES.md).
No wholesale SDK upgrade was performed.

## Sources and precedence

Current published Tencent documentation fetched over HTTPS on 2026-11-02:

- [Message overview](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/overview.html), updated 2026-07-21.
- [C2C messages](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_users_user_openid_messages.post.html).
- [Group messages](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_messages.post.html), updated 2026-10-28.
- [Webhook](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/event-emit/webhook.html) and [WebSocket](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/event-emit/websocket.html).

Reference snapshots:

- Tencent maintained [bot-docs](https://github.com/tencent-connect/bot-docs/tree/645787a45937e5d9c4f0f61afefdffde0f38696e/docs/develop/api-v2), especially `server-inter/message/send-receive/{send,event,rich-media}.md`, `dev-prepare/interface-framework/{event-emit,sign}.md`.
- Official [BotGo](https://github.com/tencent-connect/botgo/tree/fe31c0dfe469001e0f783d2f07e7de7bd08b403f), compared with vendored `dto`, `openapi/v1`, `interaction/{signature,webhook}`, `token/token_source.go`.

The old `/server-inter/message/send-receive/send.html` URL redirects to a general
startup page. It must not be treated as evidence that the old GitHub Markdown is
the current published protocol. The **current published pages take precedence**.

| Topic | Current protocol / implementation |
| --- | --- |
| Text | POST `/v2/users/{user_openid}/messages` or `/v2/groups/{group_openid}/messages`; explicitly serialize `msg_type:0`. |
| Association | Message reply uses incoming `d.id` as `msg_id`; envelope `id` is only `event_id`. Sequence starts at 1; duplicate msg_id + msg_seq fails. |
| Reply count | Published overview and C2C API introduction say **C2C 4**, group **5**. GitHub source says C2C 5: use the published stricter limit. Attempts, including timeouts, consume a sequence. |
| Lifetime | Published overview/C2C introduction: C2C 60 min, group 5 min; C2C field description still says 5 min. This is a documentation inconsistency. Capabilities exist only during synchronous event handling, send promptly, let platform enforce original-message age. No long-lived reply feature is claimed. |
| Event replies | Envelope `id` in `event_id`, only documented FRIEND_ADD/C2C_MSG_RECEIVE/GROUP_ADD_ROBOT/GROUP_MSG_RECEIVE with known source/target. INTERACTION_CREATE schema needs specialized routing and acknowledgment; acknowledge/normalize but do not grant reply capability here. |
| Media | `/files` with explicit `srv_send_msg:false`, then type 7 `/messages` with association and sequence. Preserve opaque `file_info` exactly, whether base64 or not; keep legacy []byte SDK fields source-compatible. Existing base64 upload extension retained, no new upload API introduced. Space content when otherwise empty retains old media compatibility. |
| Markdown/buttons | Type 2 + markdown object; omit content when markdown exists. Keyboard object alongside markdown. Native markdown/keyboard still depend on application permission. |
| Active sends | **Current published pages list active-send quotas and permissions.** Old GitHub source has a 2025-04-21 discontinued-push notice and older monthly quotas. Do not apply that historical notice as a blanket current prohibition. Preserve explicit manual sends, without msg_id/event_id/msg_seq; surface platform denial/limit errors without passive fallback. No is_wakeup support added. |
| Response | Published API uses RFC3339 timestamp; old source describes integer timestamp. Accept both. Require a message ID on send success and file_info on upload success; inspect HTTP and business code. |
| Webhook | HTTPS + exact raw-body Ed25519 verification, op13 challenge, op12 d=0 reception ACK / d=1 retry (vendored GenDispatchACK). Once admitted, downstream failure does not request replay. Stopped/unavailable admission retries. Signature timestamp is mandatory; no authentication bypass. |
| WebSocket | Historical GitHub source warns retirement by end of 2024; current published site retains gateway documentation without that notice. This does not establish application authorization. Keep explicit compatibility mode and mock tests, default new configurations to Webhook; actual availability unverified. Resume uses bare token as documented, Identify includes scheme. |

## Code findings

Association was missing from all group/C2C helpers; zero msg_type was omitted;
media paths duplicated upload logic, including an incompatible multipart response
envelope. HTTP-only error checking ignored business errors on 2xx. SDK client
mutated shared authorization and TraceID was unsynchronized. Signature code could
verify empty content when timestamp was missing. Core already installed a safe
SDK logger, but SDK formatted private bodies before discarding them. Narrow
patches address these without upgrading the dependency or replacing the EventBus.
