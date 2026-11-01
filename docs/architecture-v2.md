# ShizukuBot v2 — P0 architecture

## Inspection and scope

Baseline main: e86178d97724d72ec0e4e552f67de3fcedc7f238. Source, not the empty README, defines the workflow: Go 1.26, CGO SQLite, local BotGo replacement, prebuilt MDUI/LiteGraph assets. Root initially has no tests; `go test ./...` fails vet at core/bot.go's interaction format string.

Confirmed causes before implementation:
- Every Bot.Start registers method closures into BotGo event.DefaultHandlers. Last registration wins for every WS and HTTP callback. The local session manager has no cancellation; Stop cancels token refresh but leaves connections/reconnect workers alive.
- ExecutionEngine fingerprints group/channel + author + content, omits BotID, and keeps an immortal cleanup goroutine. Consecutive legitimate messages collide.
- core directly constructs graph EventContext and holds ExecutionEngine. Sandbox hardcodes bot identity and only replaces some reply callbacks.
- Startup prints credentials; config GET returns the session signing secret. Cookies lack SameSite and HTTPS policy. WS permits every Origin. Mutations lack CSRF protection. Code scanning accepts arbitrary paths, and plugin IDs become file paths.

## Direction

Transport (QQ WS or signed webhook) -> normalized events.Event -> Dispatcher (identity dedup) -> Bus -> builtin handler / legacy plugin adapter. Core owns connection and message capabilities, not graph nodes. SDK token retrieval remains lazy; the unsafe background refresh helper is not used. SDK OpenAPI/DTO stay pinned; the application owns a small context-bound gateway instead of using SDK global event callbacks or its uncancellable session manager. No vendored SDK changes or SDK migration in P0.

Event fields include bot, platform event and message IDs, canonical type, source, actor/target, original JSON and a typed reply capability. Raw JSON is an opaque compatibility envelope; public events contain no GraphData/NodeData. Different transports normalize with the same function.

Bus handlers are synchronous, registration ordered, isolated by optional BotID/type filters. Concurrent publishers may run concurrently; handlers must synchronize shared state. Each handler has a cooperative context deadline. Errors and panics are collected and later handlers still run. No goroutine is abandoned to pretend a timeout can interrupt arbitrary Go code. Future middleware and command/priority routing can wrap handlers without graph dependencies. Unsubscription is idempotent; cancellation is checked before invocation.

Dedup uses bot + source + target + actor + message ID for messages; group AT/plain and guild AT/plain aliases represent the same message. Nonmessage keys use bot + exact type + event ID. Missing message IDs fall back to exact event IDs without guessing cross-type equivalence. No reliable identity means no dedup. Entries have TTL, a capacity bound and lazy expiry; no background cleanup process. Dedup is at-most-once within the window, not durable exactly-once delivery or transaction retry.

Legacy JSON, converters, storage, node executors and frontend remain. The legacy adapter owns graph translation and capability mapping; sandbox uses capture-only reply capabilities, never a real Bot sender. Interpreted Go, file and HTTP nodes remain trusted administrator code, **not an OS security sandbox**. Cooperative deadlines cannot terminate arbitrary interpreted/native code.

Security uses explicit secure-cookie configuration or actual TLS, never forwarded headers; SameSite=Lax and HttpOnly. Mutation protection rejects foreign Origin/Referer/Fetch-Metadata and requires JSON for origin-less API mutations, retaining native JSON clients. Same-origin scripts need no UI redesign. Log streaming and administrative APIs require admin accounts (no full RBAC). Filesystem scan access is rooted below data; secrets are write-only API fields.

## Validation and follow-up

See v2-p0-report.md for actual tests, SDK compatibility findings and limits. Real QQ authorization, gateway limits, resume behavior and hosted HTTPS termination still require deployment testing. P1 should add an isolated worker protocol, explicit plugin capabilities and an SDK independent of graph structures, not expose Yaegi as isolation.
