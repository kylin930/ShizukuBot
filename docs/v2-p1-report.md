# ShizukuBot v2 P1 交付报告

## 范围与提交

基线为远程 main 的 `1f4b827cdbd7a779b9a7777a45cb0725e8ef0382`。开始时工作区干净，新建 `feat/v2-p1-plugins`，未修改、合并或推送 main，未发布云环境。

内部按四步交付：

1. P1-A `0d68ce5`：管理锁粒度、Bot 配置快照、内置回复锁外调用、独立插件元数据/Go 接口和生命周期基础。
2. P1-B `983bd7a`：可信 Go 注册、Node IPC Worker、安装存储、TS SDK 和三个可运行示例。
3. P1-C `ef9af59`：备份/迁移、代码插件 API/UI、旧市场停用、删除 LiteGraph 与旧图引擎。
4. P1-D：本报告所在提交；补齐备份恢复、关闭期间调试取消、注册/配置校验、跨平台检查、回归与开发文档。

## 实现

### 并发与核心

所有 `/api` 请求共用的 apiMu 已删除。管理配置通过独立 RW 锁取得副本，修改只在保存相关配置时加锁；同 Bot 的生命周期编辑单独串行。查询、沙盒和包扫描不持有全局配置锁。

内置指令先取得 Bot 快照，再读取命令表、获取系统信息和网络回复；初始认证也已移出 Bot 状态锁。公开 Send 方法和事件回复均向 SDK/HTTP 请求传播 context，不持 Bot 锁等待网络。新增“阻塞沙盒不阻塞统计查询”“回复取消传播”“内置回复不持 Bot 锁”的回归。P0 的事件、去重、Webhook 和 gateway 没有重新引入全局业务回调。

### 新插件模型与 Go

`plugins/` 独立于 SDK DTO、节点或编辑器。manifest 使用 API major 1、插件自身版本、entry、runtime、permissions 和 config_schema。生命周期包括 Load/Start、事件处理、Stop/Unload。注册在 Start 后封闭，同实例重复命令被拒绝；不同插件/Bot 的注册互不覆盖。

Go 扩展由工厂返回全新实例，通过静态 `RegisterBuiltins` 注册；已提供实际可用的 builtin-info（`/framework`），不使用 `.so`。Go 初始化 context 与 Host.Context() 长期生命周期分离，扩展负责合作取消和释放自己的 goroutine。

### JS/TS Worker

Node.js 24 LTS 子进程，每插件/Bot 一份。启动、事件、能力调用和停止通过有版本/ID/parent 的 JSON-lines IPC v1 交互。Go 主机验证注册、配置、权限、回复和状态，子进程不继承核心凭证环境，错误/崩溃/超时不会直接终止核心。

SDK 提供指令/参数解析、规范事件监听、事件身份及平台数据、文本/图片/富媒体/Markdown/按钮/视频回复、配置、日志和持久状态。TS 使用固定 5.9.3 构建为 ESM JS，服务器不运行 TS 编译器。examples/plugins/hello、keywords、counter 均有源码、manifest 和可直接运行的构建产物。

默认事件 2 秒，失败终止 Worker；后续匹配事件按有上限退避重建。启停/重载释放订阅；核心关闭取消并等待调试与生产实例，拒绝新 Worker。stdout 专供 IPC，stderr/console 在日志权限下有界采集。临时状态的调试实例最多四个并发。

### 数据与 UI

新项目在 `data/code-plugins/<id>`；配置、信任、启用、状态在 `data/plugin-data`，按 Bot 隔离。ZIP 使用 staging 验证、路径/大小/数量/符号链接/重复名检查后安装，已有 ID 不覆盖，不执行安装 scripts。配置支持明确的 JSON Schema 子集、默认值、类型/必填/枚举/数值校验。

代码导出与私有数据备份分开；显式恢复配置与状态到空数据 scope，保持禁用/未信任，不覆盖已有数据。卸载先停止所有实例并撤销信任，保留数据。Go 编译扩展只能禁用，不能通过上传替换或卸载。

保留 MDUI 2 导航、统计、机器人、日志、用户、认证、配置、API 测试和沙盒页面；只替换插件区域。新页面支持列表、manifest、按 Bot 配置、信任、启停/重载、本地 ZIP、导出/私有备份/恢复、运行错误和现有沙盒。不引入 React 或新视觉设计。生产路由保留核心内置命令优先级，普通插件不会额外响应已处理的内置命令；独立沙盒保持纯插件调试语义。补齐动态属性转义，避免未启用插件的元数据通过 HTML 属性执行脚本。

旧市场安装页面明确标为格式不兼容并停用；server 的配置响应标记 legacy-unsupported / installation_supported=false。未开发新市场服务。

## LiteGraph 清理验收

| 项目 | 结果 |
| --- | --- |
| litegraph.js-release/ | 已从 Git 和文件系统删除 |
| plugin-nodes.js、LGraph/LGraphCanvas、旧画布逻辑/专属 CSS | 已删除 |
| LiteGraph JS/CSS 加载、/litegraph/ 路由 | 已删除；真实 HTTP 返回 404 |
| ExecutionEngine、NodeExecutor、图运行类型/存储、Yaegi 节点 | 已删除整个旧 plugin/ 包 |
| code2graph / graph2code / code scan / graph save / custom-nodes API | 注册和实现已删除，按原 HTTP 方法测试均返回 404 |
| Yaegi / Playwright / SQLite 图状态依赖及无用间接依赖 | go mod tidy 清理；最终 go list -deps 无这些运行依赖 |
| 节点 JSON 为主要存储、WebUI 生成/保存图 | 不再存在 |
| 历史 JSON | 仅 migration 离线工具读取；不作为生产回退 |

原 P0 图专用测试源完整保存在 `migration/testdata/p0-adapter-tests.go.txt` 供审查，不参与运行时。其场景迁移到可执行的新测试：简单 JSON 加载/回复和去重对照在 migration；路径/安装安全、Worker 崩溃与取消在 plugins；全部媒体捕获在 API。其他 P0 的 core/events/gateway/webhook/auth 测试保留；原 API 测试保留名称并针对新代码包和退役 API 调整断言，没有通过删除失败断言来宣称兼容。

## 旧数据迁移结果

删除代码前先构建带 CLI 的新程序，对当前环境执行 dry-run 和 apply。当前 `data/plugins` 没有旧插件，结果 **0 个用户插件转换、0 个用户插件删除**；验证备份在：

`data/legacy-backups/20261101-072200-3160975960/`

测试数据验证了单一事件/命令到固定回复，以及可证明的 contains/equals 单线 true 条件；完整原始 JSON/custom_nodes 备份后才安装生成项目。生成项目默认禁用/未信任。重复执行不覆盖现有项目；重复 ID、自定义覆盖、额外分支、代码和复杂图进入人工清单。journal 支持安装/报告更新间中断后的核对回滚，用户编辑过的生成代码拒绝删除。

不能可靠自动转换的能力：多分支/多入口/循环、多动作/数据依赖、Go 代码、自定义节点、复杂模板、旧文件/HTTP/列表数据库/HTML 渲染等。媒体能力已在新 SDK 提供，但本轮没有自动转换这些节点。旧命令匹配通过生成的普通 JS 事件监听保留，因此需人工改写为 commands 后才会出现在新 `/help` 注册命令表。没有以隐藏编辑器或保留旧引擎代替移除。

## 测试与证据

Linux amd64，Go 1.26.10，Node v24.19.0，TypeScript 5.9.3。必要定向测试先执行，再运行根模块完整 race；失败均定位后重跑，未跳过测试或关闭断言。

```sh
export PATH=/workspace/.tools/go1.26.10/bin:$PATH
export GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOMAXPROCS=4
npm ci --ignore-scripts
npm run build:examples
go test -race -count=1 -json ./...
go test -race -count=1 ./api

go build -o /tmp/shizukubot-p1 .
go build -o /tmp/shizuku-http-p1 ./cmd/concurrency-test
python3 scripts/p0-smoke.py --binary /tmp/shizukubot-p1 --runner /tmp/shizuku-http-p1
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/shizukubot-p1.exe .
(cd botgo-master && go test -count=1 ./dto ./event ./interaction/signature)
(cd server && go build -o /tmp/shizuku-market-p1 .)
node --check webui/static/js/app.js
node --check webui/static/js/community.js
go list -deps ./...
git diff --check
```

| 检查 | 实际结果 |
| --- | --- |
| 最终根模块 race | 51 项顶层测试通过；0 失败、0 跳过，无 race 报告 |
| 退役 API 原方法断言 | 按原方法重跑 API race，通过；graph 用 PUT、custom-nodes 用 GET 检查 404 |
| JS/TS/Go 插件 | 真实 Node 加载、三例、指令、事件、多插件、多 Bot、状态重启、Go 生命周期通过 |
| 故障/安全 | 崩溃后重建、超时终止、在途禁用与关闭、协议拒绝、权限/环境过滤、配置/包、备份恢复通过 |
| P0 回归 | 去重、隔离、重连、签名、认证/CSRF/Cookie、WS、沙盒场景通过 |
| 实际主程序 | 无 LiteGraph 目录启动、登录、管理 API、静态 app.js、脱敏和 SIGTERM 退出通过 |
| HTTP 并发工具 | 10 并发、100 请求：100 成功、0 失败；测试代码包卸载成功，检查真实 HTTP/业务结果 |
| Windows | amd64 交叉构建成功，未在 Windows 主机执行 |
| 内置 SDK | DTO、event、interaction/signature 三包通过 |
| server | 独立模块构建成功，未连接 OSS 或远程认证 |
| TS/JS/依赖 | 三例 TS 编译、两个后台 JS 语法、差异格式、运行依赖清理检查通过 |

机器可读根测试在 `/tmp/shizuku-p1-final-tests.json`，依赖清单在 `/tmp/shizuku-p1-dependencies.txt`；最终实际冒烟临时目录 `/tmp/shizuku-p0-lbv4j50j`（工具为延续 P0 脚本名，测试的是新代码运行时）。临时目录含本地随机测试凭据，不应上传。

## 兼容、未验证与风险

- 保留多 Bot、群/C2C/频道/私信等事件，原有文本和富媒体核心方法、内置命令、用户认证、日志/统计和沙盒。频道/私信回复目前以文本为支持范围，其它媒体返回明确错误；真实 QQ 连接与授权媒体未测试，使用 Mock 和捕获能力验证。
- 旧数据格式不是直接兼容的生产格式，必须迁移或人工重写；安全信任、包和配置接口属于 P1 的有意变化，不宣称旧插件全部自动兼容。
- Node 独立进程不是强安全沙盒。可信 Node 代码仍能使用标准库文件/网络和创建其他进程，可能绕过 IPC 能力自行联网；heap 上限不是 RSS/OS 配额，进程树强隔离尚未实现。未经审核社区代码不默认执行。
- SDK TokenSource 的 Token() 不接收 context，首次认证/刷新仍可能等待其有界 HTTP 超时，主机能力取消依赖 SDK 合作；Node 的执行超时不是所有外部调用的绝对时间保证。
- Go 扩展在进程内，panic 回收不等于可隔离 os.Exit、独立 goroutine panic 或资源耗尽；不合作的 Go 扩展可能阻塞 Stop/Close。后台任务必须遵循 Host.Context 生命周期。
- JS 生产进程与 Bot 连接独立，停止 Bot 不会继续接收事件，但 Worker 可保持空闲；释放需禁用插件或关闭主机。重启退避是事件驱动，初始启动失败需人工修复后重试。
- 配置 schema 是受限方言；没有完整 JSON Schema、IDE、自动依赖安装或自动文件 watcher。框架不对任意插件副作用提供事务回滚/持久 exactly-once。
- 无真实 QQ、生产 HTTPS、Windows 运行机或浏览器交互自动化；不把交叉编译、静态 JS 检查或 Mock 视为这些验证。后台 CDN、旧市场 OSS/认证没有做线上功能联调。
- 历史高级文件/网络/渲染/数据库能力未包装成完整新 SDK 服务；可信插件可自行实现，但旧节点定义不再执行。详见迁移矩阵，不能依赖旧引擎作为回退。
- 备份/回滚与手工替换部署应停止主机；回滚不会在运行中的进程强制卸载，也不擦除用户后续配置。私有备份含插件自己的配置/状态，应私密保存。

## P2 建议

1. 在代码插件和严格 schema 基础上实现克制的规则编辑器，不重建节点执行体系。
2. 增加 OS 级 worker/进程树隔离、资源配额、依赖审核和更细的能力策略。
3. 扩展频道/私信媒体、交互的类型化便捷 API，补真实 QQ 与 Windows/浏览器部署测试。
4. 优化分阶段升级/热重载、失败观测、处理器级幂等和状态迁移；市场协议与社区发布流程留到 P3。
