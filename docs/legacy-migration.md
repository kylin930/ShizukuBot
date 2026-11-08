# 旧插件数据备份、Go 迁移与回滚

CLI 是离线工具，不启动 QQ 或执行旧代码。先停止旧主机并保存独立备份，原始文件不会被改写或删除。

```sh
shizukubot migrate-legacy --dry-run
shizukubot migrate-legacy --apply
# --source data/plugins --custom-nodes data/custom_nodes
# --script-source data/code-plugins --destination plugins/migrated --backups data/legacy-backups
```

apply 先复制并逐文件校验旧图、自定义节点和原脚本项目到 `data/legacy-backups/<timestamp>-<random>/`，包含原始 JSON、内嵌定义、脚本源码及项目文件。`checksums.json` 保存所有文件摘要，`report.json` 记录检测和迁移状态。目录 0700、文件 0600；每类输入总量最多 64MiB，拒绝符号链接或特殊文件，超限需使用独立验证的离线备份，不会跳过数据。生产配置和持久状态仍在 `data/plugin-data`，请另行离线备份；在线导出只包括已编译插件的数据。

## 转换边界

| 原始能力 | 处理 |
| --- | --- |
| 完整单一群/C2C 事件 → 固定文本回复 | 生成普通 Go package |
| 完整单一 command → 固定文本回复 | 生成保持旧前缀、mention 与空白语义的 Go 事件监听 |
| 上述事件 → contains/equals → 唯一 true 输出 → 固定文本 | 完整链路可证明时生成 Go |
| 多入口/分支/循环/多动作、额外数据依赖、断连/重复连接 | 原始数据备份，人工迁移 |
| 自定义节点或全局定义覆盖、Go/Yaegi、复杂模板、文件/HTTP/数据库/渲染 | 原始数据备份，人工重写 |
| 媒体、频道、私信、互动等复杂旧图 | 新 SDK 保留事件/回复能力，本轮不自动转换 |
| 原脚本项目 | 原样备份，scripts 清单报告 manual-go-rewrite，不执行或伪装转换 |

全部图节点和连接必须能解释为完整规定路径。分支、事件数据端口和未解释节点不会被忽略。复杂图不会生成占位 Go 代码或声明迁移成功。

成功生成 `plugins/migrated/<id>/plugin.go`、manifest.json（元数据清单）、migration.json（来源摘要）、README.md；源码导出 `Definition() sdk.Definition`，依赖新的 Go SDK，不包含旧图执行器。目录名可含短横线，源码统一采用合法 Go 包名。非法/保留 ID 使用稳定 legacy-hash 映射，需更新 Bot 的插件选择。

生成只表示源码转换完成，不代表编译、部署、启用或 QQ 联调完成。检查备份和源码，确认 ID/配置/状态兼容后执行：

```sh
go generate ./pluginregistry
go test ./...
go build -o shizukubot .
# 重新部署并重启主程序，再在 WebUI 为指定 Bot 启用。
```

新源码被审核和编译后才进入 Registry，没有源码上传执行、运行时解释或旧引擎回退。旧 command 转成事件监听以保持历史语义，因此不会自动出现在新 `/help` 的注册命令列表；可人工改写为 Host.Command。

## 状态与回滚

图报告记录源文件、旧/新 ID、BotID、原 SHA256、能力、状态、原因和生成摘要。状态有 convertible、manual、conflict、already-migrated、conversion-pending、generated-go-source；脚本清单使用 manual-go-rewrite。相同来源重复执行不覆盖目标；已有目标、重复 ID 和用户编辑需人工处理。

apply 在写入前持久化预期摘要 journal，私有 staging 写完再改名；中断后可用 pending 记录核对回滚。发生错误先检查报告和目标，不能删除用户数据来让迁移“通过”。迁移锁遗留时先确认没有迁移进程。

```sh
shizukubot migrate-legacy --rollback /absolute/backup/path --destination plugins/migrated
# 删除未修改的生成包后，重新生成注册表、构建并部署。
go generate ./pluginregistry
```

回滚要求显式目标与报告一致，验证原始备份和生成目录摘要，拒绝删除编辑过的源码。只移除未改动生成包，保留所有原始数据及备份；不修改运行中程序、插件配置或已部署二进制。旧脚本报告的未改动输出仍可按原目标校验回滚，不能将旧脚本输出当作当前插件加载。

测试实际编译生成 Go 包并执行群/C2C/命令/条件/Unicode/去重对照，同时验证复杂图拒绝、脚本原样备份、自定义覆盖、重复检测、中断记录和回滚保护。真实用户插件迁移结果需以运行时 report.json 为准。
