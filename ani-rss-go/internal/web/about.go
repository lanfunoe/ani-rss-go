package web

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/buildinfo"
	"ani-rss-go/internal/service"
)

// handleStop 停止服务（0=重启 1=关闭）
func handleStop(c *gin.Context) {
	status, found := requiredParam(c, "status")
	if !found {
		return
	}
	var name string
	var code int
	switch strings.TrimSpace(status) {
	case "0":
		name, code = "重启", 0
	case "1":
		name, code = "关闭", 1
	default:
		fail(c, fmt.Errorf("参数 status 无效: %s", status))
		return
	}
	slog.Info(fmt.Sprintf("正在%s", name))
	go func() {
		time.Sleep(3 * time.Second)
		os.Exit(code)
	}()
	okMsg(c, nil, "正在"+name)
}

// handleAbout 查看关于信息
func handleAbout(c *gin.Context) {
	ok(c, service.GetAbout(buildinfo.Version))
}

// handleUpdate 更新程序，Go 版本需手动替换二进制
func handleUpdate(c *gin.Context) {
	fail(c, errors.New("Go 版本不支持自动更新，请手动下载新版本"))
}
