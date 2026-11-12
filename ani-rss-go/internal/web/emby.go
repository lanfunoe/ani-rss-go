package web

import (
	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/service"
)

// handleEmbyWebHook BGM 自动点格子
func handleEmbyWebHook(c *gin.Context) {
	in, okBind := bind[service.EmbyWebHook](c)
	if !okBind {
		return
	}
	service.HandleEmbyWebHook(in, clientIP(c.Request))
	ok(c, nil)
}
