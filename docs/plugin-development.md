# Go 插件开发

后端与插件均为 Go。无需脚本运行时、转译器或动态加载器。

## 最小插件

在 `plugins/hello/plugin.go` 可以看到完整示例，其核心为：

```go
package hello

import (
    "context"
    sdk "shizukubot/plugins"
)

type Plugin struct { sdk.Base }

func Definition() sdk.Definition {
    return sdk.Definition{
        Manifest: sdk.Manifest{
            APIVersion: sdk.APIVersion, ID: "hello", Name: "Ping / Pong",
            Version: "1.0.0", Runtime: "go",
            Permissions: []string{"commands.register", "message.send"},
            ConfigSchema: sdk.Schema{Type: "object"},
        },
        New: func() sdk.Extension { return &Plugin{} },
    }
}
func (*Plugin) Load(_ context.Context, host sdk.Host) error {
    return host.Command("ping", func(ctx context.Context, call sdk.Call) error {
        return call.Reply(ctx, "pong")
    })
}
```

新增包保持普通 Go 命名，导出零参数的 Definition。不要在 package init 中连接 QQ、启动后台任务或修改全局状态；初始化在实例 Load/Start 中完成，配置与状态通过 Host/Call 获取。

```sh
go generate ./pluginregistry
go test ./plugins/... ./pluginregistry
go build -o shizukubot .
```

Generator 使用 Go 标准库解析 `plugins/**` 中非测试源文件的 Definition 并生成确定性 import/注册表，跳过 testdata 和隐藏目录，不执行源码。只对审核过的本地插件运行；保持跨平台构建兼容。生成文件加入版本管理。包签名不正确或导出重复 ID 会在编译/Registry 测试中报错。

重新构建后部署新二进制并重启主程序，保留 `webui/`、配置和 `data/plugin-data`。已编译插件可随时通过 WebUI 启停、配置、查看状态/错误、备份与沙盒测试。保存配置会重启已启用实例，配置变化不需要重新编译；源码变化需要完整构建部署，插件“重启”只重新创建现有二进制中的对象。

## 示例与测试

- `plugins/hello`：指令和文本回复，单元测试直接调用 Ping，注入捕获 Reply。
- `plugins/autoreply`：监听群/C2C，按 Bot 配置 keyword/reply；空关键词不回复。
- `plugins/example`：`/count`，配置 prefix 和按 Bot 持久计数；损坏状态返回错误。
- `plugins/framework`：保留 `/framework` 核心版本信息示例。

每个包可以独立 `go test ./plugins/hello`。Host 接口可以 Mock，State 接口可以用内存实现。`pluginregistry/registry_test.go` 使用 EventBus 验证所有编译示例及多 Bot 隔离，不启动 QQ。`plugins/manager_test.go` 覆盖生命周期、权限、配置、状态、故障隔离、取消和备份。

后台资源监听 Host.Context，Stop 等待 goroutine 退出，Unload 释放最终资源。新构造函数必须返回全新实例，不共享可变全局对象。不要持有事件回复能力跨回调使用。权限帮助明确能力调用，不提供同进程安全隔离；不要编译不可信社区代码。

配置 schema、状态配额和 HTTP 行为见 [SDK](plugin-sdk.md)。新沙盒每次创建临时状态，生产计数不会变化。私有 JSON 备份包含配置/状态，恢复必须是同 ID、兼容版本的已编译插件且没有旧数据；版本间复杂状态迁移仍需插件作者处理。

当前明确接受源码变更后重新编译。不提供 `.so`、在线 Go 编译、解释器、源码热加载或进程外协议。未来外部插件若有真实需求，可另行评估纯 Go 进程协议，不属于当前 P1。
