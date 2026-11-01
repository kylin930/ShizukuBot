# 旧插件迁移与回滚

停止旧进程后执行，先做独立的文件备份。CLI 本身不启动服务、不需要 QQ 或 Node，原 JSON 永远不删除或改写。

```sh
shizukubot migrate-legacy --dry-run
shizukubot migrate-legacy --apply
# 自定义位置：--source、--custom-nodes、--destination、--backups
shizukubot migrate-legacy --rollback /absolute/backup/path --destination data/code-plugins
```

检测默认只读。apply 先复制并逐文件校验 `data/plugins` 和 `data/custom_nodes` 到 `data/legacy-backups/<timestamp>-<random>`，包含所有原始文件、内嵌 custom_nodes 和外部自定义节点。检查 `checksums.json`、`report.json` 后再审核生成的代码。备份目录权限 0700、文件 0600；最多 64MiB，超限需独立备份处理，不能跳过验证。

## 能力对照与自动转换边界

| 旧能力 | P1 处理 |
| --- | --- |
| 单一 group/c2c 事件 → 固定文本回复 | 自动生成 ordinary JS 事件处理器 |
| 单一 command → 固定文本回复 | 自动生成保持历史前缀、mention、空白语义的 JS 事件处理器；不替换为新 shell 参数解析 |
| 以上事件 → text_contains/text_equals → 唯一 true 输出 → 固定文本 | 可证明的单线触发链自动转换 |
| 多事件入口、分支、循环、多动作、额外数据输入、断连/重复连接 | 人工迁移，禁止忽略分支后伪报成功 |
| 自定义节点/全局节点覆盖了使用的能力 | 人工迁移，完整原定义备份 |
| Go/Yaegi、文件、HTTP、列表/数据库、HTML 图片、复杂模板或转换代码 | 人工迁移到受审核的代码；没有旧引擎 fallback |
| 图片、Markdown、按钮、视频 | 新 SDK 可显式回复；本轮不自动转换旧媒体节点图 |
| 频道/私信/互动/管理复杂节点 | 事件继续进入 EventBus/SDK；旧图需人工改写 |

自动转换要求全部节点和连接都能被完整解释为规定路径，固定文本来自 action/send_message.template，动作无后续分支。保留原字符串和条件语义，生成 src/index.js、dist/index.js、manifest.json、provenance migration.json。源码中的历史匹配函数是普通代码，不解析或执行图。

转换结果默认禁用且未信任；旧插件的 enabled 不会自动继承。有效旧 ID 通常保留，非法/保留 ID 映射为稳定 legacy-<hash>，须检查报告并更新 Bot 的插件选择。历史 command 转成事件监听，因此不会自动出现在新命令注册表的 `/help` 列表；可在人工审核后改写为 SDK commands。

## 报告与重复执行

每条记录包含原文件、旧/新 ID、BotID、原 SHA256、使用能力、状态和原因。状态包括 convertible、manual、conflict、already-migrated、conversion-pending、converted-disabled。无法转换不会产生半套可执行逻辑。重复 ID 需要人工处理；已存在目标永不覆盖，provenance 相同则报告已迁移。

apply 在安装前写入带预期文件摘要的 journal，安装后确认摘要。即使安装与报告更新之间中断，pending 项也能核对回滚。遇到错误须先检查备份及目标；不要删除用户文件来让迁移“通过”。迁移锁遗留时先确认没有迁移进程再处理锁文件。

rollback 必须显式指定与报告一致的目标目录，校验原备份与生成文件摘要。生成项目经修改则拒绝删除，不覆盖用户工作。原始目录一直存在，因此回滚只移除未修改的生成项目，完整原始数据仍保留。回滚前停止当前主机，并撤销/禁用已手动激活的迁移实例；工具不会修改运行中进程或当前插件配置。

本环境实际旧插件目录为空：已完成检测和空数据备份，没有虚构真实用户转换数量。测试中覆盖了简单命令、条件、Unicode 回复对照、复杂节点拒绝、精确备份、重复检测、自定义覆盖和回滚保留编辑。
