package web

import (
	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/service"
)

// handleWebUIUpload 上传 WebUI
func handleWebUIUpload(c *gin.Context) {
	header, err := c.FormFile("file")
	if err != nil {
		fail(c, err)
		return
	}
	src, err := header.Open()
	if err != nil {
		fail(c, err)
		return
	}
	defer src.Close()
	if err := service.UploadWebUI(src, header.Size, header.Filename); err != nil {
		fail(c, err)
		return
	}
	okMsg(c, nil, "WebUI 上传完成, 请刷新页面")
}

// handleWebUIDelete 删除 WebUI
func handleWebUIDelete(c *gin.Context) {
	if err := service.DeleteWebUI(); err != nil {
		fail(c, err)
		return
	}
	okMsg(c, nil, "WebUI 删除完成")
}

// handleWebUIGetUpdate 获取 WebUI 更新
func handleWebUIGetUpdate(c *gin.Context) {
	info, err := service.WebUIUpdate()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, info)
}

// handleWebUIUpdate 更新 WebUI
func handleWebUIUpdate(c *gin.Context) {
	if err := service.UpdateWebUI(); err != nil {
		fail(c, err)
		return
	}
	okMsg(c, nil, "WebUI 更新完成, 请刷新页面")
}
