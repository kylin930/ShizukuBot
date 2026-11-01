# 插件开发

## JavaScript / TypeScript

使用 Node.js 24 LTS（开发版本见 `.node-version`）、Go 1.26+。TS 编译器固定为 5.9.3，通过根目录 `npm ci --ignore-scripts` 安装，`npm run build:examples` 验证全部示例。预编译 dist 已随仓库提供，安装插件不需要 npm。

项目布局：

```text
manifest.json          # api_version / id / name / version / runtime / entry / permissions / config_schema
package.json           # type: module；不执行 scripts
src/index.ts           # 可选源码
dist/index.js          # 编译产物；entry 指向此文件
```

```ts
import {definePlugin} from '@shizukubot/sdk';
export default definePlugin({
  id:'hello',name:'Hello',version:'1.0.0',
  commands:{ping:async ctx=>{await ctx.reply('Pong!');}}
});
```

元数据实例见 `examples/plugins/hello/manifest.json`。`api_version` 必须为 1，插件版本为明确的 `major.minor.patch`，可附 prerelease；版本或导出 ID 与 manifest 不符时拒绝启用。升级需要显式停止、备份并替换项目，安装不会直接覆盖已有插件。

内置 Node loader 将 `@shizukubot/sdk` 解析到主机随 Worker 提供的 SDK，无需在已安装插件里生成 node_modules。TS 使用示例 tsconfig 的 paths 解析类型。第三方依赖需在开发时打包进可运行 JS；P1 不自动解析/安装 npm 依赖，不接受 node_modules 上传。

三个示例均可运行：`hello` 提供 ping/echo；`keywords` 按配置关键词回复群/C2C 事件；`counter` 使用每 Bot 独立的 JSON 状态，重启后计数保留。

## 打包、安装与调试

在示例目录运行：

```sh
python3 -m zipfile -c hello.zip manifest.json package.json src dist
```

不要增加顶层 `hello/` 前缀，不包含 node_modules、凭证或 `.env`。WebUI 上传限制为 1MiB；底层安装器最多 128 个条目、每文件 1MiB、总解压 4MiB。所有路径为相对路径，符号链接和跨平台特殊路径被拒绝。

安装后先填写 Bot ID，检查权限/源码，再确认信任。确认信任与启用是不同动作；可在保持生产禁用的情况下运行捕获型沙盒。沙盒每次创建新进程和临时状态，因此 counter 调试每次从 1 开始，不修改生产计数。测试原生 Go 扩展也使用独立新对象。

开发时编辑本地项目的 dist 后使用“重启 / 重载”；TS 项目先构建 src。没有自动文件 watcher。生产建议将代码项目视为不可变部署产物，变更前备份，避免进程重启前后代码不一致。

## 配置和状态

配置按插件/Bot 隔离。config_schema 支持 object、string、boolean、number、integer、array、properties、required、default、enum、minimum、maximum、items。未知字段拒绝；未实现完整 JSON Schema。保存配置后对已启用实例执行重启，新的进程/对象使用新快照。

`config.read` 控制配置传入；`state.read`/`state.write` 控制 `ctx.state.get/set`。值必须是 JSON，单值 64KiB、状态文件 1MiB；key 最长 128 字符。状态同实例串行，不提供跨插件/跨 Bot 的事务。不要把 QQ 核心凭证放入插件配置。

导出代码包用于分享；“备份代码及私有数据”包含配置和状态，须私密保管。导入备份时勾选恢复数据，只能恢复到没有旧数据的 ID，恢复不会自动信任或启用。完整离线备份应同时保存 `data/code-plugins` 和 `data/plugin-data`。

## Go 静态扩展

`plugins.Extension` 依次实现 Load、Start、HandleEvent、Stop、Unload。Load/Start 通过 Host.Command/On 注册；同实例重复指令报错，注册在 Start 完成后封闭。每个工厂必须返回全新对象，禁止多个 Bot 共用可变实例。

注册入口为 `plugins.RegisterBuiltins(manager)`，已带可运行的 `builtin-info` / `/framework` 示例。扩展必须静态编译进二进制；不使用 `.so`，Windows/Linux 接口相同。通过 `Call.Event.Reply`、Config、State、Log 使用受检查能力，不要引用 QQ SDK 的全局回调。

初始化方法须及时返回并遵守 context；后台 goroutine 保存并监听 `Host.Context()`，Stop 应等待并释放自己的资源。HandleEvent 是每次已路由调用的通用 hook，不应重复实现已注册 Command/On 的相同逻辑。Go 扩展仍在核心进程内，不能强制停止不合作的 Go 代码。

## 平台与安全

Linux 已运行真实 Node Worker 和 HTTP 集成测试。Windows 已进行 amd64 交叉构建，尚未在 Windows 机器运行；需安装 Node 24 并加入 PATH，保留本地可写临时目录，建议使用本地磁盘项目路径。

独立进程不提供强文件/网络隔离，不能防止可信代码主动读宿主文件、启动其他进程或自行联网。不要自动运行未经审核的社区插件。强 OS 隔离、依赖供应链审核和进程树资源配额是后续工作。
