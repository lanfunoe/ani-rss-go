package web

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/service"
)

// handleCalendarIcs 获取 ICS 日历
func handleCalendarIcs(c *gin.Context) {
	content := service.GenerateIcs()
	c.Header("Content-Disposition", "attachment; filename=ani-rss-calendar.ics")
	c.Header("Cache-Control", "private, max-age=3600")
	c.Data(http.StatusOK, "text/calendar; charset=utf-8", []byte(content))
	slog.Debug(fmt.Sprintf("ICS日历已生成，包含%d字节", len(content)))
}
