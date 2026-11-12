package web

import (
	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/service"
)

// handlePlayList 获取视频列表
func handlePlayList(c *gin.Context) {
	in, okBind := bind[entity.Ani](c)
	if !okBind {
		return
	}
	list, err := service.PlayList(in.URL)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}
