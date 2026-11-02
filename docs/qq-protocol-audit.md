# QQ protocol audit (2026-11-02)

Baseline: `fc4db97` (main, merged PR #3 / P2.2). Local dependency:
`github.com/tencent-connect/botgo v0.14.0 => ./botgo-master`; the replacement
contains Gensokyo-derived DTO/event/base64 upload patches (SDK_CHANGES.md).
No wholesale SDK upgrade is necessary.

Sources inspected:

- Tencent official documentation: https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/send-receive/send.html
- Tencent maintained [bot-docs](https://github.com/tencent-connect/bot-docs/tree/645787a45937e5d9c4f0f61afefdffde0f38696e/docs/develop/api-v2), especially `server-inter/message/send-receive/{send,event,rich-media}.md`, `dev-prepare/interface-framework/{event-emit,sign}.md`.
- Official [BotGo](https://github.com/tencent-connect/botgo/tree/fe31c0dfe469001e0f783d2f07e7de7bd08b403f) compared with vendored `dto/message_create.go`, `openapi/v1/{message,openapi}.go`, `interaction/{signature,webhook}`, `token/token_source.go`.

The official web and source document route describe the same API family. Keep
C2C/QQ group v2 separate from legacy guild/channel APIs. Source revisions are
recorded because the platform can change independently of this repository.

| Topic | Verified protocol and implementation decision |
| --- | --- |
| Text | POST `/v2/users/{user_openid}/messages` or `/v2/groups/{group_openid}/messages`; required `msg_type:0` must survive zero-value JSON serialization. |
| Passive message | `msg_id` is `d.id` of an incoming message, never envelope `id`. Sequence starts at 1; same msg_id + msg_seq is a duplicate. Maximum five reply attempts per message. |
| Lifetime | Source documents C2C 60 minutes, group 5 minutes. The event capability expires when synchronous dispatch finishes (earlier); platform remains authoritative for original-message expiry. |
| Event reply | Envelope `id` goes in `event_id`, only for documented `FRIEND_ADD`, `C2C_MSG_RECEIVE`, `GROUP_ADD_ROBOT`, `GROUP_MSG_RECEIVE` with known source/target. INTERACTION_CREATE requires its own schema/response handling; normalized and acknowledged, but no reply capability in this release. |
| Media | POST `/files` with `srv_send_msg:false` (explicit JSON boolean), then POST `/messages` type 7 + returned opaque `file_info` + association + unique sequence. Retain existing base64 upload extension. Type 7 uses space content when otherwise empty as documented. |
| Markdown/buttons | Type 2, markdown object; keyboard object alongside markdown. Native markdown/keyboard availability depends on platform permission; do not infer permission from serialization. |
| Active sending | Top-of-page notice: active push discontinued 2025-04-21. Older monthly quota paragraphs remain below it and are historical, not an active-send fallback. Keep public signatures, return explicit unavailable error without network traffic. |
| Webhook | HTTPS, exact raw-body Ed25519 validation, op13 challenge response, op12 d=0 reception ACK / d=1 retry. SDK GenDispatchACK and opcode documentation distinguish reception from reply delivery. Once admitted, handler failure does not request replay. Stopped/unavailable admission returns retry. |
| WebSocket | Official event-emit source warns gradual retirement by end of 2024 and recommends Webhook. Keep compatibility code/tests, never promise new-bot availability. Resume uses token without scheme as documented; Identify includes scheme. |

Observed defects: association omitted in all QQ group/C2C send helpers; zero
msg_type omitted; duplicate media implementations included a multipart route with
an incompatible response envelope; errors only checked HTTP status, dropping
business errors on 2xx; SDK client mutated shared authorization state; TraceID
was unsynchronized; missing signature timestamp could be signed as empty content.
Core already installed a safe SDK logger, but SDK itself formatted private request
and response bodies before that logger discarded them. Narrow patches remove
these hazards while preserving the SDK interface and existing event bus.
