# [CQ:member] — 群成员变动

## 说明

用于标记群成员入群/退群事件的 CQ 码。**入站**时 Gensokyo 通过 `notice` 事件的 `message` 字段推送给后端，**出站**时后端在回复中包含此 CQ 码，Gensokyo 自动处理回复逻辑。

## 格式

```
[CQ:member,type=add/remove,user_id=虚拟ID]
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `type` | string | `add` = 成员入群，`remove` = 成员离群 |
| `user_id` | int64 | Gensokyo 转化的虚拟用户 ID（可反向解析为 OpenID） |

## 入站（Gsk → 后端）

当有群成员变动时，Gensokyo 推送 `notice` 事件，`message` 字段包含 CQ 码：

```json
{
    "post_type": "notice",
    "notice_type": "group_increase",
    "sub_type": "member",
    "group_id": 821404315,
    "user_id": 3607918353,
    "message": "[CQ:member,type=add,user_id=3607918353]",
    "event_id": "GROUP_MEMBER_ADD:..."
}
```

## 出站（后端 → Gsk）

后端回复时包含 `[CQ:member]`，Gensokyo 自动处理：

### type=add（入群被动回复）

后端发送：
```
[CQ:member,type=add,user_id=3607918353]欢迎入群！
```

Gensokyo 行为：
1. 从 echo 缓存中查找该群对应的 `event_id`
2. 使用 `event_id` 进行**被动回复**（不消耗主动消息次数）
3. 清除 CQ 码，发送文本"欢迎入群！"

### type=remove（退群主动推送）

后端发送：
```
[CQ:member,type=remove,user_id=3607918353]离开了我们
```

Gensokyo 行为：
1. 退群事件无 `event_id`，无法被动回复
2. 自动转为**主动消息推送**（需群已开启主动推送权限）

## nonebot2 示例

```python
from nonebot.adapters.onebot.v11 import GroupIncreaseNoticeEvent, GroupDecreaseNoticeEvent
from nonebot.adapters.onebot.v11 import Message

@on_notice().handle()
async def handle_group_increase(bot: Bot, event: GroupIncreaseNoticeEvent):
    cq = event.message  # "[CQ:member,type=add,user_id=3607918353]"
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
