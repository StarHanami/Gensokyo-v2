package Processor

import (
	"encoding/json"
	"time"

	"github.com/hoshinonyaruko/gensokyo/config"
	"github.com/hoshinonyaruko/gensokyo/idmap"
	"github.com/hoshinonyaruko/gensokyo/mylog"
	"github.com/tencent-connect/botgo/dto"
)

// GuildMemberNotice 频道成员变动的 OneBot notice 结构
type GuildMemberNotice struct {
	PostType     string `json:"post_type"`
	NoticeType   string `json:"notice_type"`
	SubType      string `json:"sub_type,omitempty"`
	GroupID      int64  `json:"group_id"`
	UserID       int64  `json:"user_id"`
	OperatorID   int64  `json:"operator_id,omitempty"`
	Time         int64  `json:"time"`
	SelfID       int64  `json:"self_id"`
}

// ProcessGuildMember 处理频道成员变动事件
// eventType: "GUILD_MEMBER_ADD", "GUILD_MEMBER_REMOVE", "GUILD_MEMBER_UPDATE"
func (p *Processors) ProcessGuildMember(data *dto.WSGuildMemberData, eventType string) {
	if data == nil || data.User == nil {
		mylog.Printf("ProcessGuildMember: 数据为空")
		return
	}

	selfID := int64(config.GetAppID())

	// 将 guild_id 转为虚拟 group_id
	groupID, err := idmap.StoreIDv2(data.GuildID)
	if err != nil {
		mylog.Printf("ProcessGuildMember: guild_id 转换失败: %v", err)
		return
	}

	// 将 user id 转为虚拟 user_id
	userID, err := idmap.StoreIDv2(data.User.ID)
	if err != nil {
		mylog.Printf("ProcessGuildMember: user_id 转换失败: %v", err)
		return
	}

	// 操作用户（踢人时才有）
	var operatorID int64
	if data.OpUserID != "" {
		operatorID, err = idmap.StoreIDv2(data.OpUserID)
		if err != nil {
			mylog.Printf("ProcessGuildMember: op_user_id 转换失败: %v", err)
		}
	}

	now := time.Now().Unix()

	var notice GuildMemberNotice

	switch eventType {
	case "GUILD_MEMBER_ADD":
		notice = GuildMemberNotice{
			PostType:   "notice",
			NoticeType: "group_increase",
			SubType:    "member",
			GroupID:    groupID,
			UserID:     userID,
			OperatorID: operatorID,
			Time:       now,
			SelfID:     selfID,
		}
		mylog.Printf("频道成员加入: guild=%s, user=%s, nick=%s", data.GuildID, data.User.ID, data.Nick)

	case "GUILD_MEMBER_REMOVE":
		notice = GuildMemberNotice{
			PostType:   "notice",
			NoticeType: "group_decrease",
			SubType:    "member",
			GroupID:    groupID,
			UserID:     userID,
			OperatorID: operatorID,
			Time:       now,
			SelfID:     selfID,
		}
		mylog.Printf("频道成员离开: guild=%s, user=%s, nick=%s", data.GuildID, data.User.ID, data.Nick)

	case "GUILD_MEMBER_UPDATE":
		mylog.Printf("频道成员资料变更: guild=%s, user=%s, nick=%s", data.GuildID, data.User.ID, data.Nick)
		return

	default:
		mylog.Printf("ProcessGuildMember: 未知事件类型 %s", eventType)
		return
	}

	// 序列化为 map 并广播
	jsonData, _ := json.Marshal(notice)
	var outputMap map[string]interface{}
	json.Unmarshal(jsonData, &outputMap)
	outputMap["real_group_id"] = data.GuildID
	outputMap["real_user_id"] = data.User.ID

	go p.BroadcastMessageToAll(outputMap, p.Apiv2, nil)
}
