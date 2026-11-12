// Package web HTTP 服务与 REST API（Gin 实现）
package web

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
)

// Server HTTP 服务
type Server struct {
	engine *gin.Engine
	port   string
}

// NewServer 创建服务并注册路由
func NewServer(port, webDir string) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(publicAccessMiddleware())
	engine.Use(corsMiddleware())

	// 公开路由（统一使用 /api 前缀）
	api := engine.Group("/api")
	api.POST("/login", handleLogin)
	api.POST("/testIpWhitelist", handleTestIpWhitelist)
	api.Any("/ping", handlePing)
	api.GET("/custom.js", handleCustomJs)
	api.GET("/custom.css", handleCustomCss)

	api.Use(authMiddleware())
	registerAPI(api)

	ui := uiFS(webDir)
	// 路由不存在或请求方法不匹配
	engine.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusOK, entity.NotFound())
			return
		}
		serveStatic(c, ui)
	})
	return &Server{engine: engine, port: port}
}

// Run 启动 HTTP 服务
func (s *Server) Run() error {
	return s.engine.Run(":" + s.port)
}

// publicAccessMiddleware 开启仅允许内网访问时拒绝公网请求，对页面与接口均生效
func publicAccessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !publicAccessForbidden(c.Request) {
			c.Next()
			return
		}
		writeTextHTML(c, http.StatusForbidden, "禁止公网访问")
		c.Abort()
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.Get()
		if cfg.GetAllowCors() {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "*")
			c.Header("Access-Control-Allow-Headers", "*")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if authenticated(c.Request) {
			c.Next()
			return
		}
		c.JSON(http.StatusOK, entity.Forbid("登录已失效"))
		c.Abort()
	}
}

func registerAPI(api *gin.RouterGroup) {
	// ConfigController
	api.POST("/config", handleGetConfig)
	api.POST("/setConfig", handleSetConfig)
	api.POST("/clearCache", handleClearCache)
	api.POST("/trackersUpdate", handleTrackersUpdate)
	api.POST("/testProxy", handleTestProxy)
	api.POST("/downloadLoginTest", handleDownloadLoginTest)
	// BackupController
	api.GET("/exportBackup", handleExportBackup)
	api.POST("/importBackup", handleImportBackup)
	// AniController
	api.POST("/addAni", handleAddAni)
	api.POST("/setAni", handleSetAni)
	api.POST("/deleteAni", handleDeleteAni)
	api.POST("/listAni", handleListAni)
	api.POST("/updateTotalEpisodeNumber", handleUpdateTotalEpisodeNumber)
	api.POST("/batchEnable", handleBatchEnable)
	api.POST("/refreshAll", handleRefreshAll)
	api.POST("/refreshAni", handleRefreshAni)
	api.POST("/rssToAni", handleRssToAni)
	api.POST("/previewAni", handlePreviewAni)
	api.POST("/downloadPath", handleDownloadPath)
	api.POST("/importAni", handleImportAni)
	api.POST("/refreshCover", handleRefreshCover)
	// TorrentsInfosController + TorrentController
	api.POST("/torrentsInfos", handleTorrentsInfos)
	api.POST("/deleteTorrent", handleDeleteTorrent)
	// LogsController
	api.POST("/logs", handleLogs)
	api.POST("/clearLogs", handleClearLogs)
	api.GET("/downloadLogs", handleDownloadLogs)
	// AboutController
	api.POST("/about", handleAbout)
	api.POST("/stop", handleStop)
	api.POST("/update", handleUpdate)
	// NotificationController + EmbyController
	api.POST("/testNotification", handleNotificationTest)
	api.POST("/newNotification", handleNewNotification)
	api.POST("/getTgUpdates", handleGetTgUpdates)
	api.POST("/getEmbyViews", handleGetEmbyViews)
	api.POST("/embyWebHook", handleEmbyWebHook)
	// MikanController
	api.POST("/mikan", handleMikan)
	api.POST("/mikanGroup", handleMikanGroup)
	// BgmController
	api.POST("/searchBgm", handleSearchBgm)
	api.POST("/getAniBySubjectId", handleGetAniBySubjectId)
	api.POST("/getBgmTitle", handleGetBgmTitle)
	api.POST("/rate", handleRate)
	api.POST("/setRate", handleSetRate)
	api.POST("/meBgm", handleMeBgm)
	api.POST("/bgm/oauth/callback", handleBgmOAuthCallback)
	api.POST("/getThemoviedbName", handleGetThemoviedbName)
	api.POST("/getThemoviedbGroup", handleGetThemoviedbGroup)
	api.GET("/file", handleFile)
	// UploadController
	api.POST("/upload", handleUpload)
	api.POST("/uploadAndReadToBase64", handleUploadAndReadToBase64)
	api.POST("/uploadAndRead", handleUploadAndRead)
	// WebUIController
	api.POST("/webui/upload", handleWebUIUpload)
	api.POST("/webui/delete", handleWebUIDelete)
	api.POST("/webui/getUpdate", handleWebUIGetUpdate)
	api.POST("/webui/update", handleWebUIUpdate)
	api.GET("/proxyImage", handleProxyImage)
	// AniBTController + AnimeGardenController
	api.POST("/aniBT", handleAniBT)
	api.POST("/aniBTGroup", handleAniBTGroup)
	api.POST("/animeGardenList", handleAnimeGardenList)
	api.POST("/animeGardenGroup", handleAnimeGardenGroup)
	// CollectionController
	api.POST("/startCollection", handleStartCollection)
	api.POST("/previewCollection", handlePreviewCollection)
	api.POST("/getCollectionSubgroup", handleGetCollectionSubgroup)
	// PlayController
	api.POST("/playList", handlePlayList)
	// ScrapeController
	api.POST("/scrape", handleScrape)
	api.POST("/batchScrape", handleBatchScrape)
	// IcsController
	api.GET("/calendar.ics", handleCalendarIcs)
	// AfdianController
	// api.POST("/verifyNo", handleVerifyNo)
}

func bind[T any](c *gin.Context) (*T, bool) {
	var v T
	if err := c.ShouldBindJSON(&v); err != nil {
		c.JSON(http.StatusOK, entity.Error("参数错误: "+err.Error()))
		return nil, false
	}
	return &v, true
}

// requestParam 读取请求参数，查询串优先，其次表单体
func requestParam(c *gin.Context, name string) (string, bool) {
	if v, found := c.GetQuery(name); found {
		return v, true
	}
	return c.GetPostForm(name)
}

// requiredParam 读取必填请求参数，缺失时写入错误响应
func requiredParam(c *gin.Context, name string) (string, bool) {
	v, found := requestParam(c, name)
	if !found {
		fail(c, fmt.Errorf("缺少参数 %s", name))
	}
	return v, found
}

// boolParam 按 Spring 布尔转换规则读取请求参数：true/on/yes/1 与 false/off/no/0，忽略大小写与首尾空白。
// 参数缺失或为空时，required 为 true 视为缺少参数，否则取 false
func boolParam(c *gin.Context, name string, required bool) (bool, bool) {
	v, _ := requestParam(c, name)
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "on", "yes", "1":
		return true, true
	case "false", "off", "no", "0":
		return false, true
	case "":
		if !required {
			return false, true
		}
		fail(c, fmt.Errorf("缺少参数 %s", name))
		return false, false
	}
	fail(c, fmt.Errorf("参数 %s 无效: %s", name, v))
	return false, false
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, entity.OK(data))
}

func okMsg(c *gin.Context, data any, msg string) {
	res := entity.OK(data)
	res.Message = msg
	c.JSON(http.StatusOK, res)
}

func fail(c *gin.Context, err error) {
	c.JSON(http.StatusOK, entity.Error(err.Error()))
}

// handlePing 存活测试
func handlePing(c *gin.Context) {
	ok(c, nil)
}

func decodeBase64Param(value string) (string, error) {
	if value == "" || len(value) > 16<<10 {
		return "", errors.New("参数为空或过长")
	}
	data, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(value, " ", "+"))
	if err != nil || len(data) == 0 {
		return "", errors.New("Base64 参数无效")
	}
	return string(data), nil
}
