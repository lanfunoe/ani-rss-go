package web

import (
	"archive/zip"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/logsink"
	"ani-rss-go/internal/utils"
)

// handleLogs 日志
func handleLogs(c *gin.Context) {
	ok(c, logsink.Entries())
}

// handleClearLogs 清理日志
func handleClearLogs(c *gin.Context) {
	logsink.Clear()
	slog.Info("清理日志")
	ok(c, nil)
}

// handleDownloadLogs 下载日志
func handleDownloadLogs(c *gin.Context) {
	logDir := filepath.Join(config.ConfigDir(), "logs")
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `inline; filename="logs.zip"`)
	zipWriter := zip.NewWriter(c.Writer)
	defer zipWriter.Close()
	_ = filepath.WalkDir(logDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Ext(d.Name()) != ".log" {
			return nil
		}
		return utils.AddZipFile(zipWriter, path, "")
	})
}
