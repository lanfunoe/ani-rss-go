package web

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/notification"
	"ani-rss-go/internal/service"
)

// handleNotificationTest 测试通知
func handleNotificationTest(c *gin.Context) {
	in, okBind := bind[entity.NotificationConfig](c)
	if !okBind {
		return
	}
	done := make(chan bool, 1)
	go func() {
		done <- service.NotificationTest(in)
	}()
	select {
	case r := <-done:
		if !r {
			fail(c, errors.New("测试通知失败"))
			return
		}
		ok(c, true)
	case <-time.After(30 * time.Second):
		fail(c, errors.New("测试超时"))
	}
}

// handleNewNotification 新的通知
func handleNewNotification(c *gin.Context) {
	ok(c, config.NewNotification())
}

// handleGetTgUpdates 获取TG最近消息
func handleGetTgUpdates(c *gin.Context) {
	in, okBind := bind[entity.NotificationConfig](c)
	if !okBind {
		return
	}
	updates, err := notification.TgUpdates(in)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, updates)
}

// handleGetEmbyViews 获取Emby媒体库列表
func handleGetEmbyViews(c *gin.Context) {
	in, okBind := bind[entity.NotificationConfig](c)
	if !okBind {
		return
	}
	views, err := notification.EmbyViewList(in)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, views)
}
