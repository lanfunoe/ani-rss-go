package config

import (
	"cmp"

	"ani-rss-go/internal/entity"
)

// completeNotification 补全通知配置的默认值（仅补零值，不覆盖已有）
func completeNotification(n *entity.NotificationConfig) {
	n.NotificationTemplate = cmp.Or(n.NotificationTemplate, "${notification}")
	n.Retry = cmp.Or(n.Retry, 3)
	n.Sort = cmp.Or(n.Sort, 10)
	n.NotificationType = cmp.Or(n.NotificationType, entity.NotifyTypeTelegram)
	if n.StatusList == nil {
		n.StatusList = []string{entity.StatusDownloadStart.Name, entity.StatusOmit.Name, entity.StatusError.Name}
	}
	// 邮箱
	n.MailSMTPHost = cmp.Or(n.MailSMTPHost, "smtp.qq.com")
	n.MailSMTPPort = cmp.Or(n.MailSMTPPort, 465)
	n.MailImage = cmp.Or(n.MailImage, new(true))
	n.MailSSLEnable = cmp.Or(n.MailSSLEnable, new(true))
	n.MailTLSEnable = cmp.Or(n.MailTLSEnable, new(false))
	// telegram
	n.TelegramApiHost = cmp.Or(n.TelegramApiHost, "https://api.telegram.org")
	n.TelegramImage = cmp.Or(n.TelegramImage, new(true))
	n.TelegramTopicId = cmp.Or(n.TelegramTopicId, -1)
	// webhook
	n.WebHookMethod = cmp.Or(n.WebHookMethod, "POST")
	// server酱
	n.ServerChanType = cmp.Or(n.ServerChanType, "SERVER_CHAN")
	n.ServerChanTitleAction = cmp.Or(n.ServerChanTitleAction, new(true))
	// emby
	n.EmbyRefresh = cmp.Or(n.EmbyRefresh, new(false))
	if n.EmbyRefreshViewIds == nil {
		n.EmbyRefreshViewIds = []string{}
	}
	// shell
	n.AliveLimit = cmp.Or(n.AliveLimit, 10)
	// FileMove
	n.FileMoveTarget = cmp.Or(n.FileMoveTarget, "/CD2/115/Media/番剧/${title}/Season ${season}")
	n.FileMoveOvaTarget = cmp.Or(n.FileMoveOvaTarget, "/CD2/115/Media/剧场版/${title}")
	n.FileMoveDeleteOldEpisode = cmp.Or(n.FileMoveDeleteOldEpisode, new(false))
	n.FileMoveCopyModel = cmp.Or(n.FileMoveCopyModel, new(false))
	// OpenList
	n.OpenListUploadHost = cmp.Or(n.OpenListUploadHost, "http://127.0.0.1:5244")
	n.OpenListUploadPath = cmp.Or(n.OpenListUploadPath, "/115/Media/番剧/${title}/Season ${season}")
	n.OpenListUploadOvaPath = cmp.Or(n.OpenListUploadOvaPath, "/115/Media/剧场版/${title}")
	n.OpenListUploadDeleteLocalFile = cmp.Or(n.OpenListUploadDeleteLocalFile, new(false))
	n.OpenListUploadDeleteOldEpisode = cmp.Or(n.OpenListUploadDeleteOldEpisode, new(false))
	// Bark
	n.BarkServerUrl = cmp.Or(n.BarkServerUrl, "https://api.day.app")
	if n.BarkDeviceKeys == nil {
		n.BarkDeviceKeys = []string{}
	}
	n.BarkGroup = cmp.Or(n.BarkGroup, "ani-rss")
	n.BarkUseMarkdown = cmp.Or(n.BarkUseMarkdown, new(false))
	n.BarkLevel = cmp.Or(n.BarkLevel, "active")
	n.BarkVolume = cmp.Or(n.BarkVolume, 5)
	// AstrBot，密钥和目标会话由用户填写
	n.AstrBotHost = cmp.Or(n.AstrBotHost, "http://127.0.0.1:6185")
	n.AstrBotImage = cmp.Or(n.AstrBotImage, new(true))
}

// NewNotification 新建通知配置并补全默认值
func NewNotification() *entity.NotificationConfig {
	n := &entity.NotificationConfig{Enable: new(true)}
	completeNotification(n)
	return n
}
