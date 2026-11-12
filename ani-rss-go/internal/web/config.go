package web

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/download"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/service"
	"ani-rss-go/internal/task"
)

// handleCustomJs 自定义JS
func handleCustomJs(c *gin.Context) {
	noCache(c)
	js := cmp.Or(config.Get().CustomJs, "// empty js")
	c.Data(http.StatusOK, "text/javascript; charset=utf-8", []byte(js))
}

// handleCustomCss 自定义CSS
func handleCustomCss(c *gin.Context) {
	noCache(c)
	css := cmp.Or(config.Get().CustomCss, "/* empty css */")
	c.Data(http.StatusOK, "text/css; charset=utf-8", []byte(css))
}

func noCache(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}

// handleGetConfig 获取设置
func handleGetConfig(c *gin.Context) {
	ok(c, service.GetConfig())
}

// handleSetConfig 修改设置
func handleSetConfig(c *gin.Context) {
	raw, err := c.GetRawData()
	if err != nil {
		fail(c, err)
		return
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		fail(c, err)
		return
	}
	restart, err := service.SetConfig(body)
	if err != nil {
		fail(c, err)
		return
	}
	if restart {
		go task.Restart()
	}
	okMsg(c, nil, "修改成功")
}

// handleTrackersUpdate 更新trackers
func handleTrackersUpdate(c *gin.Context) {
	in, okBind := bind[entity.Config](c)
	if !okBind {
		return
	}
	if err := download.UpdateTrackers(in); err != nil {
		fail(c, err)
		return
	}
	ok(c, nil)
}

// handleTestProxy 代理测试
func handleTestProxy(c *gin.Context) {
	param, found := requiredParam(c, "url")
	if !found {
		return
	}
	target, err := decodeBase64Param(param)
	if err != nil {
		fail(c, errors.New("地址 base64 解码失败"))
		return
	}
	in, okBind := bind[entity.Config](c)
	if !okBind {
		return
	}
	data, err := service.TestProxy(target, in)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, data)
}

// handleClearCache 清理缓存
func handleClearCache(c *gin.Context) {
	size, err := service.ClearCache()
	if err != nil {
		fail(c, err)
		return
	}
	okMsg(c, nil, fmt.Sprintf("清理完成, 共清理 %s", size))
}

// handleDownloadLoginTest 下载器测试
func handleDownloadLoginTest(c *gin.Context) {
	in, okBind := bind[entity.Config](c)
	if !okBind {
		return
	}
	if !download.New(in.DownloadToolType).Login(in) {
		fail(c, errors.New("登录失败"))
		return
	}
	okMsg(c, nil, "登录成功")
}
