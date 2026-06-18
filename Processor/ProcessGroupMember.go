package Processor

import (
	"strconv"
	"time"

	"github.com/hoshinonyaruko/gensokyo/config"
	"github.com/hoshinonyaruko/gensokyo/idmap"
	"github.com/hoshinonyaruko/gensokyo/mylog"
	"github.com/tencent-connect/botgo/dto"
)

// ProcessGroupMember 处理群成员变动事件
// eventType: "GROUP_MEMBER_ADD" 或 "GROUP_MEMBER_REMOVE"
func (p *Processors) ProcessGroupMember(data *dto.GroupMemberEvent, eventType string) {
	if data == nil {
		mylog.Printf("ProcessGroupMember: 数据为空")
		return
	}

	selfID := int64(config.GetAppID())

	// 将 group_openid 转为虚拟 group_id
	groupID, err := idmap.StoreIDv2(data.GroupOpenID)
	if err != nil {
		mylog.Printf("ProcessGroupMember: group_id 转换失败: %v", err)
		return
	}

	// 将 member_openid 转为虚拟 user_id（入群/退群成员）
	userID, err := idmap.StoreIDv2(data.MemberOpenID)
	if err != nil {
		mylog.Printf("ProcessGroupMember: user_id 转换失败: %v", err)
		return
	}

	// 时间戳
	var timestamp int64
	switch v := data.Timestamp.(type) {
	case string:
		timestamp, _ = strconv.ParseInt(v, 10, 64)
	case int64:
		timestamp = v
	case float64:
		timestamp = int64(v)
	default:
		timestamp = time.Now().Unix()
	}

	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}

	switch eventType {
	case "GROUP_MEMBER_ADD":
		notice := GroupNoticeEvent{
			GroupID:     groupID,
			NoticeType:  "group_increase",
			PostType:    "notice",
			SelfID:      selfID,
			SubType:     "member",
			Time:        timestamp,
			UserID:      userID,
			RealUserID:  data.MemberOpenID,
			RealGroupID: data.GroupOpenID,
		}
		outputMap := structToMap(notice)
		mylog.Printf("群成员加入: group=%s, user=%s", data.GroupOpenID, data.MemberOpenID)
		p.BroadcastMessageToAll(outputMap, p.Apiv2, data)

	case "GROUP_MEMBER_REMOVE":
		notice := GroupNoticeEvent{
			GroupID:     groupID,
			NoticeType:  "group_decrease",
			PostType:    "notice",
			SelfID:      selfID,
			SubType:     "member",
			Time:        timestamp,
			UserID:      userID,
			RealUserID:  data.MemberOpenID,
			RealGroupID: data.GroupOpenID,
		}
		outputMap := structToMap(notice)
		mylog.Printf("群成员离开: group=%s, user=%s", data.GroupOpenID, data.MemberOpenID)
		p.BroadcastMessageToAll(outputMap, p.Apiv2, data)

	default:
		mylog.Printf("ProcessGroupMember: 未知事件类型 %s", eventType)
	}
}
