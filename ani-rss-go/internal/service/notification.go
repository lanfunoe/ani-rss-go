// Package service — 通知测试
package service

import (
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/notification"
	"ani-rss-go/internal/store"
)

// NotificationTest 以示例订阅（BGM 292970，进度 6/12）同步发送测试通知
func NotificationTest(n *entity.NotificationConfig) bool {
	ani := store.CreateAni()
	if info := bgmInfoBySubjectId("292970", true); info != nil {
		ToAni(info, ani)
	}
	ani.CurrentEpisodeNumber = 6
	ani.TotalEpisodeNumber = 12
	ani.Subgroup = entity.SubgroupUnknown
	return notification.SendTest(n, ani, "test", entity.StatusDownloadStart)
}
