# botgo-forgensokyo SDK 更改说明

> 基于 [botgo](https://github.com/tencent-connect/botgo) 官方 SDK 进行魔改，
> 参考 [botpy](https://github.com/tencent-connect/botpy) Python SDK 的完整实现。
> 主要补齐**群聊（Group）**和**C2C（好友私聊）**场景下缺失的事件、字段和数据结构。

---

## 一、事件补充（Group & C2C 管理事件）

### 1.1 新增事件类型

参考 botpy `flags.py` 中 `public_messages` Intent 下包含的完整事件列表，补齐了 6 个缺失的事件类型。

**文件：** `dto/websocket_event.go`

| 事件常量 | 事件类型字符串 | 对应 botpy 事件 | 说明 |
|---------|--------------|----------------|------|
| `EventGroupAddRobot` | `GROUP_ADD_ROBOT` | `on_group_add_robot` | 机器人加入群聊 |
| `EventGroupDelRobot` | `GROUP_DEL_ROBOT` | `on_group_del_robot` | 机器人退出群聊 |
| `EventGroupMsgReject` | `GROUP_MSG_REJECT` | `on_group_msg_reject` | 群聊关闭机器人主动消息 |
| `EventGroupMsgReceive` | `GROUP_MSG_RECEIVE` | `on_group_msg_receive` | 群聊开启机器人主动消息 |
| `EventC2CMsgReject` | `C2C_MSG_REJECT` | `on_c2c_msg_reject` | 用户关闭机器人主动消息 |
| `EventC2CMsgReceive` | `C2C_MSG_RECEIVE` | `on_c2c_msg_receive` | 用户开启机器人主动消息 |

### 1.2 Intent 映射更新

**文件：** `dto/websocket_event.go`

已将上述 6 个事件加入 `IntentGroupMessages` 的事件列表中，注册对应 Handler 时会自动订阅此 Intent。

---

## 二、新增 DTO 数据结构

### 2.1 新增文件：`dto/manage.go`

定义了群聊/C2C 管理事件的数据结构：

```go
type GroupManageData struct {
    Timestamp      uint64 `json:"timestamp,omitempty"`
    GroupOpenID    string `json:"group_openid,omitempty"`
    OpMemberOpenID string `json:"op_member_openid,omitempty"`
}

type C2CManageData struct {
    Timestamp uint64 `json:"timestamp,omitempty"`
    OpenID    string `json:"openid,omitempty"`
}
```

### 2.2 新增 Payload 类型

**文件：** `dto/websocket_payload.go`

```go
type WSGroupManageData GroupManageData
type WSC2CManageData C2CManageData
```

---

## 三、Handler 类型补充

**文件：** `event/register.go`

### 3.1 新增 Handler 类型

```go
type GroupManageEventHandler func(event *dto.WSPayload, data *dto.WSGroupManageData) error
type C2CManageEventHandler func(event *dto.WSPayload, data *dto.WSC2CManageData) error
```

### 3.2 DefaultHandlers 新增字段

```go
GroupManage GroupManageEventHandler
C2CManage   C2CManageEventHandler
```

### 3.3 注册逻辑

在 `RegisterHandlers()` 中补充了两个 Handler 的注册，并自动关联对应的 Intent：

- `GroupManageEventHandler` → `EventGroupAddRobot` / `EventGroupDelRobot` / `EventGroupMsgReject` / `EventGroupMsgReceive`
- `C2CManageEventHandler` → `EventC2CMsgReject` / `EventC2CMsgReceive`

---

## 四、事件解析函数补充

**文件：** `event/event.go`

### 4.1 eventParseFuncMap 新增映射

```go
dto.EventGroupAddRobot:   groupManageHandler,
dto.EventGroupDelRobot:   groupManageHandler,
dto.EventGroupMsgReject:  groupManageHandler,
dto.EventGroupMsgReceive: groupManageHandler,
dto.EventC2CMsgReject:    c2cManageHandler,
dto.EventC2CMsgReceive:   c2cManageHandler,
```

### 4.2 新增解析函数

```go
func groupManageHandler(payload *dto.WSPayload, message []byte) error
func c2cManageHandler(payload *dto.WSPayload, message []byte) error
```

---

## 五、字段缺失修复

### 5.1 User 结构体补充

**文件：** `dto/user.go`

| 新增字段 | JSON Tag | 说明 | 对应 botpy 字段 |
|---------|---------|------|----------------|
| `Status` | `status` | 用户状态 | `UserPayload.status` |
| `MemberOpenID` | `member_openid` | 群聊成员 openid（群聊场景）| `GroupMessage.author.member_openid` |
| `UserOpenID` | `user_openid` | C2C 用户 openid（C2C 场景）| `C2CMessage.author.user_openid` |

**重要说明：** 频道场景下用户标识是 `author.id`，但群聊/C2C 场景下使用的是 `author.member_openid` 和 `author.user_openid`。原 SDK 没有这两个字段，导致群聊/C2C 消息中无法获取发送者身份。

### 5.2 Message 结构体补充

**文件：** `dto/message.go`

| 新增字段 | JSON Tag | 说明 | 对应 botpy 字段 |
|---------|---------|------|----------------|
| `Seq` | `seq` | 全局消息序号 | `MessagePayload.seq` |

### 5.3 MediaInfo 结构体补充

**文件：** `dto/message_create.go`

| 新增字段 | JSON Tag | 说明 | 对应 botpy 字段 |
|---------|---------|------|----------------|
| `FileUUID` | `file_uuid` | 文件ID | `Media.file_uuid` |
| `TTL` | `ttl` | 有效期（秒） | `Media.ttl` |

---

## 六、Intent 补充

**文件：** `dto/websocket_intent.go`

| 新增 Intent | 值 | 对应 botpy Intent | 说明 |
|------------|---|-------------------|------|
| `IntentOpenForum` | `1 << 18` | `open_forum_event` | 开放论坛对象事件 |
| `IntentAudioOrLiveChannelMember` | `1 << 19` | `audio_or_live_channel_member` | 音视频/直播子频道成员进出事件 |

---

## 七、使用示例

### 7.1 注册群管理事件

```go
import (
    "github.com/tencent-connect/botgo/dto"
    "github.com/tencent-connect/botgo/event"
)

func main() {
    // ... 初始化 token, api ...

    _ = event.RegisterHandlers(
        // 群管理事件（进退群、主动消息权限变更）
        event.GroupManageEventHandler(func(evt *dto.WSPayload, data *dto.WSGroupManageData) error {
            switch evt.Type {
            case dto.EventGroupAddRobot:
                log.Printf("机器人被加入群聊: %s", data.GroupOpenID)
            case dto.EventGroupDelRobot:
                log.Printf("机器人被移出群聊: %s", data.GroupOpenID)
            case dto.EventGroupMsgReject:
                log.Printf("群聊 %s 关闭主动消息", data.GroupOpenID)
            case dto.EventGroupMsgReceive:
                log.Printf("群聊 %s 开启主动消息", data.GroupOpenID)
            }
            return nil
        }),

        // C2C 管理事件（主动消息权限变更）
        event.C2CManageEventHandler(func(evt *dto.WSPayload, data *dto.WSC2CManageData) error {
            switch evt.Type {
            case dto.EventC2CMsgReject:
                log.Printf("用户 %s 关闭主动消息", data.OpenID)
            case dto.EventC2CMsgReceive:
                log.Printf("用户 %s 开启主动消息", data.OpenID)
            }
            return nil
        }),
    )
}
```

### 7.2 获取群聊/C2C 消息发送者身份

```go
// 群聊消息
event.GroupATMessageEventHandler(func(evt *dto.WSPayload, data *dto.WSGroupATMessageData) error {
    senderOpenID := data.Author.MemberOpenID  // 群聊场景下使用此字段
    log.Printf("群聊消息来自: %s", senderOpenID)
    return nil
})

// C2C 消息
event.C2CMessageEventHandler(func(evt *dto.WSPayload, data *dto.WSC2CMessageData) error {
    senderOpenID := data.Author.UserOpenID    // C2C 场景下使用此字段
    log.Printf("C2C 消息来自: %s", senderOpenID)
    return nil
})
```

---

## 八、关于群聊/C2C 场景下的 mentions 说明

**重要提醒：** 官方 API 在群聊/C2C 场景下与频道场景存在差异：

| 对比项 | 频道 (Guild) | 群聊 (Group) | C2C |
|--------|-------------|-------------|-----|
| 用户标识 | `author.id` | `author.member_openid` | `author.user_openid` |
| `content` 中 @格式 | `<@!userid>` | ❌ 无包裹 | ❌ 无包裹 |
| `mentions` 数组 | ✅ 有 | ⚠️ 可能不返回 | ⚠️ 可能不返回 |
| `mention_everyone` | ✅ 有 | ❌ 无 | ❌ 无 |

**如果需要确认官方是否返回 `mentions`，请在 Handler 中打印原始 payload：**

```go
import "github.com/tidwall/gjson"

func(evt *dto.WSPayload, data *dto.WSGroupATMessageData) error {
    mentions := gjson.Get(string(evt.RawMessage), "d.mentions")
    log.Printf("mentions 原始内容: %s", mentions.Raw)
    return nil
}
```

---

## 九、变更文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `dto/websocket_event.go` | 修改 | 新增 6 个事件类型 + 更新 Intent 映射 |
| `dto/websocket_intent.go` | 修改 | 新增 2 个 Intent 常量 |
| `dto/websocket_payload.go` | 修改 | 新增 2 个 Payload 类型 |
| `dto/manage.go` | 新增 | GroupManageData / C2CManageData 结构体 |
| `dto/user.go` | 修改 | 新增 Status / MemberOpenID / UserOpenID |
| `dto/message.go` | 修改 | 新增 Seq 字段 |
| `dto/message_create.go` | 修改 | MediaInfo 新增 FileUUID / TTL |
| `event/register.go` | 修改 | 新增 2 个 Handler 类型 + 注册逻辑 |
| `event/event.go` | 修改 | 新增事件映射 + 2 个解析函数 |

---

## 十、编译与测试验证

```bash
# 编译
go build ./...

# 运行 dto 和 event 测试
go test ./dto/... ./event/...
```

所有修改均已通过编译和单元测试。
