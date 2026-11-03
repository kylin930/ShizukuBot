# P2.5 统计持久化与 WebUI 个性化报告

2026-11-03。基线 main `4d62743`，已确认 PR #6 合并 P2.4。
分支：`feat/v2-p25-stats-appearance`，未合并 main。

## 实现

| 范围 | 结果 |
| --- | --- |
| SQLite | `storage/stats` 独立存储层；`data/stats.db`、Schema v1、WAL、Busy Timeout、事务、完整性检查和明确的打开失败反馈 |
| 去重 | 按 Bot/类别/日期保存 HMAC 摘要；服务端密钥独立保存，不记录原始用户/群 OpenID、正文或凭证 |
| 统计口径 | 当前保留期内的用户、群、收发计数；各 Bot 去重后相加；DAU 按统计时区当日计算 |
| 写入 | 4096 项有限缓冲、最多 256 项事务、每秒 Flush、失败回滚和重试、关闭时排空；满队列记录错误与丢弃计数 |
| 查询 | 汇总和单 Bot API 保留；新增历史、状态与过期清理 API；查询先提交先前排队数据，不因刷新重复累计 |
| 历史图 | 真实 SQLite 日聚合，7/30/90 天、缺失日期填 0、范围控件与查询均受保留期限制；当天小时图为 24 槽 |
| 清理 | 默认 90 天、Asia/Shanghai 03:00；可选 7/30/90/180/365 天或永久保留；启动补查、在线改计划、手动只清理过期行 |
| QPS | 修复空闲读取及跨完整窗口后仍保留过期桶的问题；瞬时值不跨重启恢复 |
| 背景 | 保留原默认图；支持 HTTPS URL、安全站内图片和 PNG/JPEG/WebP 上传、预览、取消与恢复默认 |
| 动态配色 | 使用 MDUI 2.1.5 的 `setColorScheme`、`removeColorScheme`、`setTheme`；支持预设、自定义主色和三种明暗模式 |
| 初始化 | 公开启动脚本只提供外观三字段，在主体内容绘制前应用主题；登录页和管理页共享服务器持久设置 |
| 留白 | 共享 `.page-view` 内层应用桌面 32px、平板 24px、手机 16px 水平留白；保留 MDUI 对 Rail/App Bar 的原生占位 |
| 关于 | 项目介绍、后端版本/运行信息、实际 SDK 依赖版本、已核实项目链接、简洁技术栈和 Apache License 2.0 |
| 文案 | 插件/规则/沙盒的按钮、状态、空态、短提示去掉句尾句号；保留完整帮助段落、危险操作与失败反馈 |

没有修改 QQ SDK、插件市场、规则匹配语义或沙盒隔离，没有新增前端构建流程。
P2.4 的进入动画、原生导航/对话框、快速导航取消与 reduced-motion 保持。

## 存储、清理与异常

Schema、去重范围、备份方式和接口详见 [统计存储](statistics-storage.md)。
Schema v1 包含 `hourly(bot,day,hour,sent,received)`、`identities(bot,kind,day,digest)` 和 `metadata(key,value)`。
现有小时字段 `Hour/Sent/Received` 保留，并提供规范小写字段；已有汇总计数字段保持。
之前未持久化的内存统计无法追溯恢复，本轮不补造旧历史。

保留期包含今天，默认时区为 Asia/Shanghai。时区与数据库绑定，避免新旧日期分桶混用；清理时间可在线更新。
计划按本地日历日期判断，记录每天的尝试日期；覆盖月界、春季跳时和秋季重复小时测试。
启动补查和手动清理与计划任务分开，不通过新增定时器累积任务。清理不执行全库 VACUUM。
删除 Bot 后其数据不参与总体统计；相同 ID 重建会恢复保留期内的同一逻辑 Bot 历史。

正常负载下未提交窗口约 1 秒或一个批次；异常退出会失去尚未提交的内存记录。
失败时最多保留缓冲与批次共 4352 项，数据库恢复后重试；满队列明确记录丢弃，不阻塞 QQ 分发。
设置页可见 ready/degraded、文件大小、最近清理、错误和丢弃记录；读取/清理失败不会伪报成功。
`BenchmarkBatch` 实测 30378 ns/op，约 3.3 万次“接收 + 用户去重”操作/秒，属于本环境局部基准，并非 QQ 吞吐保证。

依赖核实：SQLite `v1.61.1`（2026-10-31 发布，要求 Go 1.26.0），WebP 解码使用 `golang.org/x/image v0.47.0`。
配套 libc/mathutil/memory 等间接依赖由 Go Modules 固定；botgo-plus 仍为 `v0.0.0-20261102144408-979b74884c88`。
无 CGO Windows amd64 交叉构建通过，无 GCC 依赖。

## 配置与外观

旧配置缺少新字段时使用兼容默认值，不删除 Bot、指令模板或已有 Secret。
保存使用临时文件、Sync 和原子替换；WebUI 带版本标识，拒绝并发旧快照覆盖，失败保留草稿。
旧客户端省略版本标识仍兼容，新客户端应发送 `revision`。
原始全局 JSON 可保存 `statistics` 与 `webui.appearance`；凭证继续不回填，Bot 通过机器人页管理。
若文件已保存但运行中策略应用失败，明确提示部分完成。

上传由现有管理员认证及同源检查保护，实际解码后校验格式、8 MiB、边长 4096 和 1600 万像素限制。
随机文件名、不使用原始上传路径，拒绝 SVG、路径穿越和非图片；图片位于独立数据目录。
图片 GET 公开供登录页加载；上传/删除/修改配置均受保护。
保存新背景时删除旧上传，取消时清理临时上传；下次上传回收超过 24 小时的未引用文件，并限制最多 32 个文件。
同一上传 URL 用于预览和保存。普通表单遇到图片加载失败会保留服务器配置，页面也提供默认图回退与恢复入口。
外部 URL 由浏览器加载，后端不代抓取，不引入 URL 抓取 SSRF。

公开 `/api/appearance` 和启动脚本仅含 `background_url/primary_color/mode`，不访问或暴露管理员完整配置。
服务器持久化后，新浏览器和登录页一致；其他已打开页面刷新后使用新设置。
统计图使用主题的 Primary/Tertiary 和前景/网格色；跟随系统时系统明暗变化也会更新图表颜色。
MDUI 两个文档页面在本环境请求返回 403；三个官方全局函数已通过实际 2.1.5 构建源码及 Chromium 调用确认。

关于页的 Core/WebUI 版本来自后端 `core/version.go` 的真实声明，目前均为 1.0.0；前端没有硬编码该版本，也未将其改称新发布版本。
Go、平台、运行时间来自运行时；SDK 版本来自构建依赖信息。六个项目/文档/许可链接均核实返回 200。

## 真实截图

使用隔离的本地数据库、禁用 Bot 和真实运行的 WebUI，截图来自 Chromium。
历史样本由测试专用 Go helper 写入 SQLite，再由正常 API 查询；不代表真实 QQ 收发。
上传样图为本地 256×144 PNG 测试文件，只用于隔离上传验收；默认背景仍为 P2.4 原图。
MDUI 主内容区内部滚动，设置与关于页图片记录当时的可见区域，统计分组单独滚动后截图。

| 场景 | 截图 |
| --- | --- |
| 默认背景、真实日/小时图、保留期口径 | [统计桌面](images/p2-5/stats-desktop.png) |
| 上传预览、蓝色主题、深色模式 | [外观桌面](images/p2-5/appearance-desktop.png) |
| 保留期与清理状态 | [统计设置](images/p2-5/statistics-desktop.png) |
| 实际版本、SDK 和项目链接 | [关于](images/p2-5/about-desktop.png) |
| 1440×900 设置 | [截图](images/p2-5/config-1440.png) |
| 1024×768 设置 | [截图](images/p2-5/config-1024.png) |
| 390×844 设置 | [截图](images/p2-5/config-390.png) |
| 360×800 设置 | [截图](images/p2-5/config-360.png) |
| 新浏览器登录页的相同外观 | [手机深色登录](images/p2-5/login-mobile-dark.png) |
| 手机 Drawer | [截图](images/p2-5/drawer-mobile.png) |

## 验证

| 检查 | 结果 |
| --- | --- |
| `go test ./...` | 通过 |
| `go test -race ./...` | 通过 |
| `go vet ./...` | 通过 |
| `go build ./...` | 通过 |
| `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...` | 通过 |
| `scripts/p0-smoke.py` | 通过：启动、认证、静态资源、凭证隐藏、100/100 并发 HTTP、优雅退出 |
| `scripts/p2-browser-smoke.py` | 通过：插件表单/JSON、Bot 隔离、草稿、实例操作、沙盒、六种回复、权限/XSS/失败夹具 |
| `scripts/p22-browser-smoke.py` | 通过：规则创建/编辑/启停/删除取消、筛选、预览、隔离、版本冲突、迟到响应、认证与 XSS |
| `scripts/p23-browser-smoke.py` | 通过：诊断无证据状态、客户端送达区别、桌面/手机、转义夹具 |
| `scripts/p24-browser-smoke.py` | 通过：立即/可取消进入、21 次快速导航、历史同步、旧请求/计时器清理、对话框焦点/Esc、防重保存与提示、reduced-motion |
| `scripts/p25-browser-smoke.py` | 通过：真实历史/进程重启、在线保留策略、清理和原始 JSON 保存、URL/上传/默认、主题/模式、刷新与新浏览器登录一致、预览取消回滚、系统明暗图表同步 |

SQLite 单元测试覆盖首次创建、重启去重、收发/小时/日历史、多 Bot、DAU 跨天、并发写入、事务中途回滚、只读写入失败重试、有限缓冲、保留期/永久保留、并发清理、调度更新、月界/DST、不可写目录、密钥缺失、损坏/较新 Schema、关闭 Flush。
事务回滚测试将故障语句固定在同一写入批次，100 次重复及 20 次 race 重复验证通过。
API 测试覆盖配置冲突与保存失败、凭证保留、公开数据最小化、上传/错误文件/大小/尺寸、非法 URL、恢复默认删除旧文件、实际存储策略应用和 Bot 删除/重建归属。

11 个路由 × 4 档视口共 44 组布局检查：文档、主内容容器及内部视图均无横向溢出。
浏览器捕获的未处理异常为空，数据见 [verification.json](images/p2-5/verification.json)。
动效、主题切换和恢复行为通过实际操作及运行时断言验证，没有用固定等待掩盖生命周期问题。

复现专项（Python Playwright + 系统 Chromium，Go 1.26+）：

```sh
go build -o /tmp/shizukubot-p25 .
go build -o /tmp/p25-seed-stats scripts/p25-seed-stats.go
python3 scripts/p25-browser-smoke.py --binary /tmp/shizukubot-p25 --seed-helper /tmp/p25-seed-stats --artifacts /tmp/p25-screenshots
```

## 边界

未发现本轮验收遗留的功能或未处理 JavaScript 错误。
Windows 为交叉构建验证，未运行 Windows 实机；手机为 Chromium 视口验证，未测手机硬件帧率。
没有连接真实 QQ；真实平台验收结论沿用已有 QQ 联调文档，不将本地统计样本或沙盒成功作为平台送达证据。
数据库/上传属于单进程本地数据，未实现多节点共享存储或复杂分析；默认图片、MDUI 和字体仍依赖原外部服务。

使用与配置说明见 [WebUI 设置](webui-guide.md)。
