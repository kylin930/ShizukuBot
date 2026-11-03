# P2.4 WebUI 打磨报告

2026-11-03；基线为 main `2f29b3c`，包含 P2.3 QQ 修复和 P2.3.1 botgo-plus 迁移。
开发分支：`feat/v2-p24-webui-expressive-polish`。本轮只修改前端、浏览器回归和文档；Go API、SDK 依赖、规则语义和沙盒隔离未变。

## 页面与文案

| 页面 | 调整 |
| --- | --- |
| 统计 | 保留数据、图表与机器人概览；数字使用等宽数字，刷新图表不重复播放动画 |
| 机器人 | 启停按钮明确显示请求中和禁用状态；平台/API 报错显示失败；诊断收敛为接入方式、最近事件/回复/错误 |
| QQ 诊断 | 技术字段放入“详细诊断”，权限、Webhook 和客户端送达说明放入“接入帮助”；无证据显示“暂无记录/未检测” |
| 插件 | 删除编译机制的顶部说明；重启帮助、备份限制放在操作旁；保留策略排除、异常和部分成功提示 |
| 规则 | 两段常驻机制说明移入“规则说明”；保存、启停和测试反馈简短，保留范围、字节限制和重复回复提醒 |
| 沙盒 | 默认只显示“仅模拟执行，不会发送 QQ 消息。”；范围细节可展开；追加结果保留已有回合 DOM |
| 社区 | “旧版插件暂不支持。”和开发文档链接；部署说明可展开 |
| 配置 | 字段就近校验、加载失败提示、未保存修改保护；凭证不回填，预览不输出已有密钥 |
| 关于 | 移除无法从 API 确认的固定前端版本号；SDK 显示 botgo-plus |
| 登录 | 保留主体布局；必填、认证和网络错误清晰，登录中禁止重复提交 |

删除、丢弃修改仍使用原生确认框；保存成功只显示一次 Snackbar；字段错误和持续失败分别留在表单与 Notice。
完整对照见 [文案审查](webui-copy-review.md)。

## 动效与生命周期

实测基线 `@2` 加载 MDUI **2.1.5**，现将 CSS/JS 固定到该版本。以下变量通过浏览器计算样式确认存在，原始值见 [tokens.json](images/p2-4/after/tokens.json)。

| Token | 实测值 | 使用 |
| --- | --- | --- |
| `--mdui-motion-duration-short3` | 150ms | 局部详情/结果进入、按钮按压、列表选中、边框变化 |
| `--mdui-motion-duration-medium1` | 250ms | 页面进入 |
| `--mdui-motion-duration-medium2` | 300ms | 确认可用；未额外增加长动画 |
| `--mdui-motion-easing-standard` | `cubic-bezier(0.2, 0, 0, 1)` | 按钮、列表和边框过渡 |
| `--mdui-motion-easing-emphasized` | `cubic-bezier(0.2, 0, 0, 1)` | 确认可用；保留原生组件动效 |
| `--mdui-motion-easing-emphasized-decelerate` | `cubic-bezier(0.05, 0.7, 0.1, 1)` | 页面及局部进入 |

路由立即创建内容和发起请求，去掉原先的 180ms 渲染等待和 500ms 退出过渡。
页面内容从 opacity 0.78、Y 轴 6px 进入；局部详情为 3px；按钮按压缩放 0.985，不改变布局。
原生导航、Ripple 和 Dialog 保持原有行为；展示型统计卡片不加悬浮抬升。

快速导航会取消旧动画、请求、统计定时器、图表和日志连接；异步结果受页面代次/作用域约束。
MDUI 文本框存在卸载后异步尺寸计算的时序问题：旧视图先隐藏并设为 inert，待组件原生更新结束及下一帧再移除；新页面渲染不等待旧视图。
浏览器专项验证连续 21 次导航后只保留一个视图，无旧统计定时器和日志连接。

`prefers-reduced-motion: reduce` 关闭新增 CSS 过渡、WAAPI 进入效果及图表进入动画；切换设置会取消正在播放的装饰动画。
缺少 `Element.animate` 时直接显示内容；不依赖 View Transitions，不引入动画库、全局 will-change 或持续装饰动画。

## 视觉与背景

保留原配色、Navigation Rail、Drawer、App Bar 和页面排版。
将组件整体 opacity 改为半透明 Surface：卡片 0.93、App Bar 0.96、Rail 0.92、Drawer 0.98，前景保持 opacity 1。
MDUI RGB 通道变量均按 `rgb(var(...))` / `rgba(var(...), alpha)` 使用。
手机端使用滚动背景，修正表单、卡片操作和文本换行。

背景 URL 保持原值：
`https://teachermate.oss-cn-qingdao.aliyuncs.com/vokMW-1750340487715-nekoaft_N02_a.png`。
基线截图中该资源因来源策略被浏览器以 `ERR_BLOCKED_BY_ORB` 拒绝，呈现纯色底；诊断确认同 URL 使用 `no-referrer` 可返回 200 并解码。
后台与登录模板改用带 `referrerpolicy="no-referrer"` 的装饰背景图元素，保持 cover/居中和原 URL；不响应点击，也不进入无障碍内容。修改后截图先等待原图成功解码再采集，尺寸 1920×1080，见 [背景记录](images/p2-4/after/background.json)。
未替换、重绘或生成背景图片。

## 真实截图

均来自 Linux Chromium 运行中的 WebUI。修改前截图在基线源码尚未编辑时采集；两组使用相同的隔离本地 Bot、已存储规则和实际沙盒响应。
测试 Bot 均禁用，截图不包含生产凭证；后续失败/XSS 响应夹具与截图采集分开。
桌面为 1440×900，手机为 390×844、规则窄屏为 360×800；长页面保留完整截图。

| 页面 | 修改前 | 修改后 |
| --- | --- | --- |
| 统计 | [截图](images/p2-4/before/stats-desktop.png) | [截图](images/p2-4/after/stats-desktop.png) |
| 机器人 | [截图](images/p2-4/before/bots-desktop.png) | [截图](images/p2-4/after/bots-desktop.png) |
| 插件 | [截图](images/p2-4/before/plugins-desktop.png) | [截图](images/p2-4/after/plugins-desktop.png) |
| 规则 | [截图](images/p2-4/before/rules-desktop.png) | [截图](images/p2-4/after/rules-desktop.png) |
| 沙盒 | [截图](images/p2-4/before/sandbox-desktop.png) | [截图](images/p2-4/after/sandbox-desktop.png) |
| QQ 诊断展开 | [截图](images/p2-4/before/qq-diagnostics-desktop.png) | [截图](images/p2-4/after/qq-diagnostics-desktop.png) |
| 移动导航 | [截图](images/p2-4/before/navigation-mobile.png) | [截图](images/p2-4/after/navigation-mobile.png) |
| 360px 规则 | [截图](images/p2-4/before/rules-mobile-360.png) | [截图](images/p2-4/after/rules-mobile-360.png) |
| 配置 | [截图](images/p2-4/before/config-desktop.png) | [截图](images/p2-4/after/config-desktop.png) |
| 关于 | [截图](images/p2-4/before/about-desktop.png) | [截图](images/p2-4/after/about-desktop.png) |
| 手机登录 | [截图](images/p2-4/before/login-mobile.png) | [截图](images/p2-4/after/login-mobile.png) |

未制作录屏。动效通过运行时动画对象、计算样式、媒体设置切换和实际操作验证，不以静态截图判定。

## 验证结果

| 检查 | 结果与范围 |
| --- | --- |
| `go test ./...` | 通过 |
| `go test -race ./...` | 通过 |
| `go vet ./...` | 通过 |
| `go build ./...` | 通过 |
| `GOOS=windows GOARCH=amd64 go build ./...` | 通过 |
| `scripts/p0-smoke.py` | 通过；HTTP 100/100、认证、静态资源、凭证隐藏、正常退出 |
| `scripts/p2-browser-smoke.py` | 通过；插件配置/JSON 往返、Bot 隔离、脏表单、迟到响应、实例操作、沙盒与六种结构化回复；另测 Schema/XSS/失败响应 |
| `scripts/p22-browser-smoke.py` | 通过；规则创建/编辑/删除/取消/启停、搜索筛选、Bot 隔离、预览、冲突、草稿和 XSS；另测认证/空态/失败 |
| `scripts/p23-browser-smoke.py` | 通过；真实诊断无证据状态、客户端送达区别、桌面/手机；另测转义夹具 |
| `scripts/p24-browser-smoke.py` | 通过；立即渲染、实际 250ms 进入和旧动画取消、静态回退、迟到请求清理、21 次快速导航与历史同步、列表节点保留、真实保存防重和单次提示、原生对话框 Esc/焦点恢复、规则预览、连续沙盒回复、配置校验与草稿保护、Bot 失败反馈、reduced-motion、两种手机宽度逐页无横向溢出、文本注入防护、登录错误 |

专项数据见 [verification.json](images/p2-4/after/verification.json)；页面及局部进入动画在减少动态效果时均为 0，按钮/列表/卡片的新增 transition-duration 为 0s。
[未处理异常记录](images/p2-4/after/browser-errors.json) 为空。
保存防重使用被明确挂起的真实保存响应，迟到响应使用事件屏障；未用固定等待掩盖生命周期问题。

复现（使用本地构建的二进制）：

```sh
python3 scripts/p2-browser-smoke.py --binary /tmp/shizukubot-p24
python3 scripts/p22-browser-smoke.py --binary /tmp/shizukubot-p24
python3 scripts/p23-browser-smoke.py --binary /tmp/shizukubot-p24
P24_PHASE=after python3 scripts/p24-browser-smoke.py --binary /tmp/shizukubot-p24 --artifacts docs/images/p2-4/after
```

## 未解决问题与验证边界

本轮浏览器验收未发现尚未解决的功能或 JavaScript 错误。移动端为 Chromium 视口检查，未测手机实机帧率。
未连接真实 QQ：QQ 诊断来自实际本地 API 的无事件状态，Bot 启动失败响应另用明确夹具测试；沙盒捕获回复不作为平台送达证据。
背景图、MDUI 和图标仍依赖现有外部服务。
