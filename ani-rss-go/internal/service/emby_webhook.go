// Package service — Emby WebHook：播放后自动在 BGM 点格子
package service

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/download"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/items"
	"ani-rss-go/internal/rename"
	"ani-rss-go/internal/store"
)

// EmbyWebHook Emby 通知内容，字段名大小写不敏感
type EmbyWebHook struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Date        string `json:"date"`
	Event       string `json:"event"`
	Severity    string `json:"severity"`
	User        struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"user"`
	Server struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"server"`
	Item struct {
		Path       string `json:"path"`
		SeriesName string `json:"seriesName"`
		FileName   string `json:"fileName"`
	} `json:"item"`
	PlaybackInfo struct {
		PlayedToCompletion bool `json:"playedToCompletion"`
	} `json:"playbackInfo"`
}

// embyWebHookQueue 串行处理点格子任务，队列满时丢弃
var embyWebHookQueue = make(chan func(), 256)

func init() {
	go func() {
		for job := range embyWebHookQueue {
			job()
		}
	}()
}

// HandleEmbyWebHook 处理 Emby 事件，按播放状态标记 BGM 剧集
func HandleEmbyWebHook(hook *EmbyWebHook, clientIP string) {
	slog.Debug(fmt.Sprintf("webhook: %+v", *hook))
	if strings.TrimSpace(config.Get().BgmToken) == "" {
		slog.Info("BgmToken 为空")
		return
	}
	if slices.Contains([]string{"system.webhooktest", "system.notificationtest"}, hook.Event) {
		slog.Info(fmt.Sprintf(`接收到测试请求:
====================================
IP: %s
ServerId: %s
ServerName: %s
ServerVersion: %s
====================================`, clientIP, hook.Server.ID, hook.Server.Name, hook.Server.Version))
		return
	}

	fileName := hook.Item.FileName
	m := entity.SeasonReg.FindStringSubmatch(fileName)
	if m == nil {
		return
	}
	season, _ := strconv.Atoi(m[1])
	// 番外
	if season < 1 {
		return
	}
	episode, _ := strconv.ParseFloat(m[2], 64)
	// 集 x.5
	if items.Is5(episode) {
		return
	}
	collectionType := embyCollectionType(hook)
	if collectionType < 0 {
		// 播放状态未正确获取
		return
	}

	job := func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Emby WebHook 点格子异常", "fileName", fileName, "error", r, "stack", string(debug.Stack()))
			}
		}()
		slog.Info(fmt.Sprintf("%s 标记为 [%s]", fileName, []string{"未看过", "想看", "看过"}[collectionType]))
		// 优先匹配路径相同的
		subjectId := ""
		for _, ani := range store.ANIList() {
			if embyEqualsAni(hook, ani, season) {
				subjectId = GetSubjectId(ani)
				break
			}
		}
		if subjectId == "" {
			subjectId = BgmSubjectIdByName(hook.Item.SeriesName, season)
		}
		episodeId := BgmEpisodeId(subjectId, episode)
		if episodeId == "" {
			slog.Info("获取bgm对应剧集失败")
			return
		}
		slog.Debug("subjectId: " + subjectId)
		slog.Debug("episodeId: " + episodeId)
		if err := BgmCollections(subjectId); err != nil {
			slog.Error(err.Error())
		}
		if err := BgmCollectionsEpisode(episodeId, collectionType); err != nil {
			slog.Error(err.Error())
		}
	}
	select {
	case embyWebHookQueue <- job:
	default:
		slog.Warn("Emby WebHook 队列已满，已丢弃", "fileName", fileName)
	}
}

// embyEqualsAni 按下载路径、标题、TMDB 名称判断事件是否对应订阅
func embyEqualsAni(hook *EmbyWebHook, ani *entity.Ani, season int) bool {
	if season != ani.Season || ani.BgmUrl == "" {
		return false
	}
	if filepath.Clean(download.GetDownloadPath(ani)) == filepath.Dir(hook.Item.Path) {
		// 路径相同
		return true
	}
	seriesName := hook.Item.SeriesName
	if rename.Del(ani.Title, false) == seriesName {
		// 名称与季相同
		return true
	}
	if ani.Tmdb == nil {
		return false
	}
	if ani.Tmdb.Name != "" && ani.Tmdb.Name == seriesName {
		return true
	}
	return ani.Tmdb.OriginalName != "" && ani.Tmdb.OriginalName == seriesName
}

// embyCollectionType 获取播放状态：0 未看过，2 看过，-1 未知
func embyCollectionType(hook *EmbyWebHook) int {
	switch strings.ToLower(hook.Event) {
	case "item.markunplayed":
		return 0
	case "item.markplayed":
		return 2
	case "playback.stop":
		if hook.PlaybackInfo.PlayedToCompletion {
			return 2
		}
	}
	return -1
}
