# [CQ:member] — 群成员变动

## 说明

用于标记群成员入群/退群事件的 CQ 码。`group_id` 和 `user_id` 均为 Gensokyo 对 OpenID 转换后的虚拟 ID。

## 格式

```
[CQ:member,type=add/remove,group_id=虚拟群ID,user_id=虚拟用户ID]
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `type` | string | `add` = 成员入群，`remove` = 成员离群 |
| `group_id` | int64 | Gensokyo 转化的虚拟群 ID（出站时 Gsk 反向转为 OpenID 以确定目标群） |
| `user_id` | int64 | Gensokyo 转化的虚拟用户 ID（可反向解析为 OpenID） |

## 整体流程

### type=add（成员入群）

```
① Gsk 捕获 GROUP_MEMBER_ADD 事件
     ↓
② Gsk 向 后端 推送 notice，message 包含 group_id 和 user_id
   例: [CQ:member,type=add,group_id=821404315,user_id=3607918353]
     ↓
③ 后端 解析 CQ 码，取出 type、group_id、user_id
     ↓
④ 后端 向 Gsk 推送消息（包含 [CQ:member] + 文本）
   例: [CQ:member,type=add,group_id=821404315,user_id=3607918353]欢迎入群！
     ↓
⑤ Gsk 解析 CQ 码，将 group_id 转回 GroupOpenID 确定目标群
   将 user_id 转回 OpenID，使用 event_id 进行**被动回复**，发送文本
```

### type=remove（成员退群）

```
① Gsk 捕获 GROUP_MEMBER_REMOVE 事件
     ↓
② Gsk 向 后端 推送 notice，message 包含 group_id 和 user_id
   例: [CQ:member,type=remove,group_id=821404315,user_id=3607918353]
     ↓
③ 后端 解析 CQ 码，取出 type、group_id、user_id
     ↓
④ 后端 向 Gsk 推送消息（包含 [CQ:member] + 文本）
   例: [CQ:member,type=remove,group_id=821404315,user_id=3607918353]离开了呢
     ↓
⑤ Gsk 解析 CQ 码，将 group_id 转回 GroupOpenID 确定目标群
   将 user_id 转回 OpenID，无 event_id，直接**主动消息发送**
```

## 入站详情（Gsk → 后端）

Gensokyo 捕获群成员变动事件后，向后端推送 `notice` 事件，`message` 字段仅包含纯 CQ 码，无其他内容：

```
[notice.group_increase.member]: [CQ:member,type=add,group_id=821404315,user_id=3607918353]
[notice.group_decrease.member]: [CQ:member,type=remove,group_id=821404315,user_id=3607918353]
```

## 出站详情（后端 → Gsk）

后端回复时在消息中包含 `[CQ:member]` 加上要发送的文本，Gensokyo 自动解析 CQ 码参数并处理。

### type=add

后端发送：
```
[CQ:member,type=add,group_id=821404315,user_id=3607918353]欢迎入群！
```

Gensokyo 处理：
1. 移除 `[CQ:member]` CQ 码
2. 将 `group_id=821404315` 反向转换为 GroupOpenID，作为目标群
3. 将 `user_id=3607918353` 反向转换为 OpenID
4. 从缓存查找该群对应的 `event_id`
5. 使用 `event_id` 进行**被动回复**，发送文本"欢迎入群！"

### type=remove

后端发送：
```
[CQ:member,type=remove,group_id=821404315,user_id=3607918353]离开了呢
```

Gensokyo 处理：
1. 移除 `[CQ:member]` CQ 码
2. 将 `group_id=821404315` 反向转换为 GroupOpenID，作为目标群
3. 将 `user_id=3607918353` 反向转换为 OpenID
4. 退群无 `event_id`，直接进行**主动消息发送**，发送文本"离开了呢"

## nonebot2 示例

```python
from nonebot.adapters.onebot.v11 import GroupIncreaseNoticeEvent, GroupDecreaseNoticeEvent
from nonebot.adapters.onebot.v11 import Message

@on_notice().handle()
async def handle_group_increase(bot: Bot, event: GroupIncreaseNoticeEvent):
    cq = event.message  # "[CQ:member,type=add,group_id=821404315,user_id=3607918353]"
    await bot.send_group_msg(
        group_id=event.group_id,
        message=Message(f"{cq}欢迎新成员！")
    )

@on_notice().handle()
async def handle_group_decrease(bot: Bot, event: GroupDecreaseNoticeEvent):
    cq = event.message
    await bot.send_group_msg(
        group_id=event.group_id,
        message=Message(f"{cq}离开了我们")
    )
```

## 配置

需在 `config.yml` 的 `text_intent` 中启用：

```yaml
text_intent:
  - "GroupMemberAddEventHandler"
  - "GroupMemberRemoveEventHandler"
```

## 适用范围

🏷️ 群聊
