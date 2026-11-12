package web

import (
	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/service"
)

// handleMikan 获取Mikan番剧列表
func handleMikan(c *gin.Context) {
	text, found := requiredParam(c, "text")
	if !found {
		return
	}
	in, okBind := bind[service.MikanSeason](c)
	if !okBind {
		return
	}
	ok(c, service.MikanSearch(text, in))
}

// handleMikanGroup 获取Mikan番剧的字幕组列表
func handleMikanGroup(c *gin.Context) {
	url, found := requiredParam(c, "url")
	if !found {
		return
	}
	ok(c, service.MikanGroupList(url))
}
