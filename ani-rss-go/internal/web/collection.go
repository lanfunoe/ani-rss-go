package web

import (
	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/service"
)

// handleStartCollection 开始下载合集
func handleStartCollection(c *gin.Context) {
	in, okBind := bind[service.CollectionInfo](c)
	if !okBind {
		return
	}
	if err := service.StartCollection(in); err != nil {
		fail(c, err)
		return
	}
	okMsg(c, nil, "已经开始下载合集")
}

// handlePreviewCollection 合集预览
func handlePreviewCollection(c *gin.Context) {
	in, okBind := bind[service.CollectionInfo](c)
	if !okBind {
		return
	}
	list, err := service.PreviewCollection(in)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// handleGetCollectionSubgroup 获取合集字幕组
func handleGetCollectionSubgroup(c *gin.Context) {
	in, okBind := bind[service.CollectionInfo](c)
	if !okBind {
		return
	}
	subgroup, err := service.CollectionSubgroup(in)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, subgroup)
}
