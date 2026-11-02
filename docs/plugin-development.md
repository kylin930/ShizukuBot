# Go 插件开发

## 五分钟创建第一个插件

仓库的 `plugins/hello/plugin.go` 已提供 Ping 示例。普通插件只需导出 Definition：

```go
package hello

import (
    "context"
    "shizukubot/sdk"
)

func Definition() sdk.Definition {
    return sdk.CommandPlugin(
        sdk.Meta{ID: "hello", Name: "Ping / Pong", Version: "1.0.0"},
        "ping",
        func(ctx context.Context, c sdk.Call) error {
            return c.Reply(ctx, "pong")
        },
    )
}
```

不必手写结构体、工厂、Load 或空生命周期。API 版本、Go runtime、空对象配置和指令/回复权限由构造器填写。Version 必须显式提供，不推断默认版本。新增其他插件时改用唯一 ID 和 Go 包名；不要在 package init 中连接 QQ 或启动任务。

```sh
go generate ./pluginregistry
go test ./sdk ./plugins/... ./pluginregistry
go build -o shizukubot .
```

部署新二进制并重启主程序，在 MDUI 2 的插件页面选择 Bot 并启用；沙盒选择 hello，输入 `/ping` 即可捕获 pong，不需要真实 QQ。启停和配置无需编译；源码变更后须生成注册表、测试、重新构建并部署，仅点“重启插件”不会加载修改后的源码。

独立测试可以直接调用处理函数并注入 `sdk.Call{Event: sdk.Event{Reply: ...}}`，参考 `plugins/hello/plugin_test.go`；也可用 `Definition().New()` 和 Mock Host 测试注册。SDK 本身只依赖 Go 标准库和公共事件类型，不依赖管理器或 QQ SDK。

## 监听消息事件

```go
func Definition() sdk.Definition {
    return sdk.EventPlugin(
        sdk.Meta{ID: "welcome", Name: "Welcome", Version: "1.0.0"},
        []sdk.EventType{sdk.GroupMessage, sdk.GroupAtMessage, sdk.C2CMessage},
        func(ctx context.Context, c sdk.Call) error {
            if c.Event.Message == "你好" {
                return c.Reply(ctx, "你好！")
            }
            return nil
        },
    )
}
```

GroupMessage 与 GroupAtMessage 对应不同规范推送类型，按需要都订阅；规范化机制没有更改。EventPlugin 只自动声明事件订阅和回复权限。使用 `c.Event.BotID/UserID/GroupID/Message` 取得身份和内容，指令参数为 `c.Args`，没有另一份重复事件状态。

`c.ReplyMarkdown(ctx,text)`、`c.ReplyImage(ctx,url)` 和 `c.Send(ctx,sdk.Reply{...})` 共用原回复能力，错误必须返回。群/C2C 支持核心已有媒体，频道/私信当前只支持文本，其他类型返回 unsupported。不要保存 Reply 跨事件使用，须传播调用 context。

## 使用配置

为构造器增加一个明确选项：

```go
sdk.WithConfig(sdk.Schema{
    Type: "object",
    Properties: map[string]sdk.Schema{
        "keyword": {Type: "string", Default: []byte(`"你好"`)},
        "reply":   {Type: "string", Default: []byte(`"你好！"`)},
    },
})
```

WithConfig 同时授予 config.read。事件回调读取 `c.Config["keyword"]`、`c.Config["reply"]`，完整可运行例子见 `plugins/autoreply`，仍使用 EventPlugin。配置和状态按插件/Bot 隔离，未知字段拒绝。WebUI 保存配置会重启已启用实例以应用新快照，无需重新编译。

Schema 支持 object、string、boolean、number、integer、array、properties、required、default、enum、minimum、maximum、items；是有限方言，不是完整 JSON Schema。不要放入核心凭证。

## 多个指令与持久化状态

有多个指令/监听器时，用 Define 在每个实例 Load 中注册，保留错误返回：

```go
return sdk.Define(sdk.Meta{ID: "tools", Name: "Tools", Version: "1.0.0"},
    func(r sdk.Registrar) error {
        if err := r.Command("ping", Ping); err != nil {
            return err
        }
        return r.On(sdk.C2CMessage, Observe)
    },
)
```

Define 默认授予指令注册、事件订阅和回复；不授予配置、状态或日志。Registrar 只在这次回调内有效，回调结束后封闭；重复/无效注册会令 Load 失败，即使误忽略了某次注册的错误也不会变成成功。初始化失败仍进入 Manager 的清理和错误记录。

需要持久化状态时显式增加 `sdk.WithPermissions("state.read", "state.write")`，使用 `c.State.Get/Set(ctx,key,json.RawMessage)`。缺失 key 返回 nil，单值最多 64KiB、每插件/Bot 状态文件 1MiB，key 最长 128 字符。状态在禁用、重启后保留；沙盒每次使用临时状态，不改生产数据。

构造器每次生成新的 Extension。固定处理函数应不含共享可变状态；若要实例内的短期变量，在 Define 的注册回调内部创建闭包。SDK 不会复制调用者闭包捕获的全局变量或外部指针。需要资源和更复杂状态时使用 Advanced API。

## 高级生命周期

`plugins/example` 的计数器保留显式 Manifest、工厂和 `sdk.Base`，演示生命周期为何有用：

- Load 保存当前 Host，注册绑定当前实例的 Count 方法。
- Start 读取配置与持久计数，损坏状态返回错误，按 Bot 缓存到当前对象。
- Count 先持久化再更新内存和回复；写入失败不推进计数。
- Stop 停止使用，Unload 释放 Host 引用；新实例从磁盘恢复。

复杂插件可覆写 Load、Start、HandleEvent、Stop、Unload。后台 goroutine 监听 `Host.Context()`，Stop 等待任务结束并释放资源；初始化 context 只用于初始化，不应作为长期任务的生命周期。清理可能发生在部分初始化失败后，须允许幂等/部分初始化。

实例内事件和生命周期串行，不同实例可并行。error/panic 记录到状态并继续其他处理器，默认超时是合作取消，不强杀 Go goroutine。原生可信插件同进程运行，SDK 权限及调试沙盒不是文件/网络安全隔离；自行创建的 goroutine 要自行同步和处理 panic。

## SDK 参考与部署边界

完整类型、构造器默认值、校验时机和 HTTP 接口见 [Go SDK](plugin-sdk.md)。`sdk` 定义公共开发契约；`plugins.Native` 是当前原生适配器；`plugins.Manager/Registry` 管理生命周期和静态目录。它们没有分裂成两个运行时。

新增包在 `plugins/**` 导出 `Definition() sdk.Definition`，`go generate ./pluginregistry` 按确定性顺序生成静态 imports，不执行插件源码；生成文件应提交 Git。重复 ID 在 Registry 校验时返回错误。源码签名不正确由 Go 编译器报告。

保留 `webui/`、配置和 `data/plugin-data` 完成部署。线上数据备份为私有 JSON，恢复必须是兼容的已编译插件且没有旧数据/实例，恢复后禁用。高级插件持久状态的版本升级由作者处理。旧 `plugins` 公共类型是 deprecated 的 sdk 别名，可渐进替换 import；没有重复实现两套模型。

本轮不实现 `.so`、RPC/IPC、热编译、文件监听、源码上传或自动下载。未来进程外插件是否兼容同步接口需独立设计，当前不作无修改复用承诺。实际验证见 [P1.2 报告](v2-p1-sdk-dx-report.md)。
