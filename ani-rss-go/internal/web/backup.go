package web

import (
	"archive/zip"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/buildinfo"
	"ani-rss-go/internal/service"
	"ani-rss-go/internal/task"
)

// handleExportBackup 导出备份
func handleExportBackup(c *gin.Context) {
	filename := fmt.Sprintf("ani-rss.backup.%s.zip", buildinfo.Version)
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))
	if err := service.WriteBackup(c.Writer); err != nil {
		slog.Error("导出设置失败", "error", err)
	}
}

// handleImportBackup 导入备份
func handleImportBackup(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		fail(c, err)
		return
	}
	if strings.TrimPrefix(filepath.Ext(file.Filename), ".") != "zip" {
		fail(c, errors.New("导入格式异常"))
		return
	}
	src, err := file.Open()
	if err != nil {
		fail(c, err)
		return
	}
	defer src.Close()
	reader, err := zip.NewReader(src, file.Size)
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.ImportBackup(reader); err != nil {
		fail(c, err)
		return
	}
	go task.Restart()
	okMsg(c, nil, "导入成功")
}
