# [CQ:member] — 群成员变动

## 说明

用于标记群成员入群/退群事件的 CQ 码。`group_id` 和 `user_id` 均为 Gensokyo 对 OpenID 转换后的虚拟 ID。

入站事件使用标准 OneBot V11 通知格式（`notice.group_increase` / `notice.group_decrease`），`message` 字段中附带 CQ 码供后端解析。出站仍为普通 `send_group_msg`。

## 格式

```
[CQ:member,type=add/remove,group_id=虚拟群ID,user_id=虚拟用户ID]
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `type` | string | `add` = 成员入群，`remove` = 成员离群 |
| `group_id` | int64 | Gensokyo 转化的虚拟群 ID |
| `user_id` | int64 | Gensokyo 转化的虚拟用户 ID |

## 流程

### type=add（成员入群）

```
① Gsk 捕获 GROUP_MEMBER_ADD → 推送 notice 通知
   [notice.group_increase.approve]: {"message":"[CQ:member,type=add,group_id=821404315,user_id=3607918353]",...}

② 后端收到通知 → 用 send_group_msg 回复
   send_group_msg(group_id=821404315, message="[CQ:member,type=add,group_id=821404315,user_id=3607918353]欢迎入群！")

③ Gsk 收到消息 → 解析 CQ 码，group_id 转 GroupOpenID 确定目标群
   user_id 转 OpenID，用 event_id 被动回复，发送"欢迎入群！"
```

### type=remove（成员退群）

```
① Gsk 捕获 GROUP_MEMBER_REMOVE → 推送 notice 通知
   [notice.group_decrease.leave]: {"message":"[CQ:member,type=remove,group_id=821404315,user_id=3607918353]",...}

② 后端收到通知 → 用 send_group_msg 回复
   send_group_msg(group_id=821404315, message="[CQ:member,type=remove,group_id=821404315,user_id=3607918353]离开了呢")

③ Gsk 收到消息 → 解析 CQ 码，group_id 转 GroupOpenID 确定目标群
   user_id 转 OpenID，无 event_id，直接主动消息发送"离开了呢"
```

## 后端示例（nonebot2）

> **重要**: 必须使用 `on_notice` 而非 `on_message`。Gensokyo 以标准 OneBot V11 通知格式（`notice.group_increase` / `notice.group_decrease`）推送事件，`[CQ:member]` 在 `message` 字段中。

### 合并处理器（推荐）

```python
from nonebot import on_notice
from nonebot.adapters.onebot.v11 import (
    Bot, GroupIncreaseNoticeEvent, GroupDecreaseNoticeEvent, Message
)

member_handler = on_notice(priority=1, block=False)

@member_handler.handle()
async def handle_member(bot: Bot, event: GroupIncreaseNoticeEvent | GroupDecreaseNoticeEvent):
    """统一处理群成员入群/退群"""
    cq_code = getattr(event, "message", "") or ""

    if isinstance(event, GroupIncreaseNoticeEvent):
        await bot.send_group_msg(
            group_id=event.group_id,
            message=Message(
                f"{cq_code}"
                f"[CQ:at,qq={event.user_id}]"
                f"[CQ:markdown,data=<base64>]"  # 替换为实际 markdown
            )
        )
    else:
        await bot.send_group_msg(
            group_id=event.group_id,
            message=Message(f"{cq_code}离开了我们")
        )
```

### 分离处理器（可选）

```python
from nonebot import on_notice
from nonebot.adapters.onebot.v11 import (
    Bot, GroupIncreaseNoticeEvent, GroupDecreaseNoticeEvent, Message
)

@on_notice().handle()
async def handle_group_increase(bot: Bot, event: GroupIncreaseNoticeEvent):
    cq_code = getattr(event, "message", "")
    await bot.send_group_msg(
        group_id=event.group_id,
        message=Message(
            f"{cq_code}"
            f"[CQ:at,qq={event.user_id}]"
            f"[CQ:markdown,data=<base64>]"
        )
    )

@on_notice().handle()
async def handle_group_decrease(bot: Bot, event: GroupDecreaseNoticeEvent):
    cq_code = getattr(event, "message", "")
    await bot.send_group_msg(
        group_id=event.group_id,
        message=Message(f"{cq_code}离开了我们")
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
