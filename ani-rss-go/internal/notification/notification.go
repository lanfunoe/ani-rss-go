// Package notification 消息通知
// 分发模型：单 goroutine 顺序发送，按通知配置的顺序逐个渠道分发
package notification

import (
	"cmp"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/download"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/rename"
)

type sendTask struct {
	cfg    *entity.Config
	ani    *entity.Ani
	text   string
	status entity.NotificationStatus
}

var queue = make(chan sendTask, 256)

func init() {
	// 单 goroutine 顺序发送
	go func() {
		for t := range queue {
			send(t.cfg, t.ani, t.text, t.status)
		}
	}()
}

// sender 通知渠道的发送实现，发送成功时返回 true
type sender func(n *entity.NotificationConfig, ani *entity.Ani, text string, status entity.NotificationStatus) bool

// senders 通知类型对应的渠道
var senders = map[string]sender{
	entity.NotifyTypeEmbyRefresh:    embyRefreshSend,
	entity.NotifyTypeMail:           mailSend,
	entity.NotifyTypeServerChan:     serverChanSend,
	entity.NotifyTypeSystem:         systemSend,
	entity.NotifyTypeTelegram:       telegramSend,
	entity.NotifyTypeWebHook:        webHookSend,
	entity.NotifyTypeShell:          shellSend,
	entity.NotifyTypeFileMove:       fileMoveSend,
	entity.NotifyTypeOpenListUpload: openListUploadSend,
	entity.NotifyTypeBark:           barkSend,
	entity.NotifyTypeAstrBot:        astrBotSend,
}

// Send 异步发送通知
func Send(cfg *entity.Config, ani *entity.Ani, text string, status entity.NotificationStatus) {
	if ani != nil && !ani.GetMessage() {
		return
	}
	select {
	case queue <- sendTask{cfg, ani, text, status}:
	default:
		slog.Warn("通知队列已满，丢弃本次通知")
	}
}

// send 按 sort 升序逐个发送已启用且包含该状态的通知，失败时间隔一秒重试
func send(cfg *entity.Config, ani *entity.Ani, text string, status entity.NotificationStatus) {
	list := slices.SortedStableFunc(slices.Values(cfg.NotificationConfigList), func(a, b entity.NotificationConfig) int {
		return cmp.Compare(a.Sort, b.Sort)
	})
	for _, n := range list {
		if !n.GetEnable() || !slices.Contains(n.StatusList, status.Name) {
			continue
		}
		fn, ok := senders[n.NotificationType]
		if !ok {
			// 通知类型可能已经被删除
			continue
		}
		for attempt := range max(n.Retry, 1) {
			if attempt > 0 {
				time.Sleep(time.Second)
				slog.Warn(fmt.Sprintf("通知失败 正在重试 第%d次 %s", attempt, n.NotificationType))
			}
			if sendOne(fn, &n, ani, text, status) {
				break
			}
		}
	}
}

// sendOne 调用渠道发送，渠道内的 panic 视为发送失败
func sendOne(fn sender, n *entity.NotificationConfig, ani *entity.Ani, text string, status entity.NotificationStatus) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("通知渠道异常", "type", n.NotificationType, "error", r)
		}
	}()
	return fn(n, ani, text, status)
}

// replaceTemplate 通知模版替换
func replaceTemplate(ani *entity.Ani, n *entity.NotificationConfig, text string, status entity.NotificationStatus) string {
	t := cmp.Or(n.NotificationTemplate, "${notification}")
	comment := n.Comment
	if strings.TrimSpace(comment) == "" {
		comment = "无备注"
	}
	t = strings.ReplaceAll(t, "${comment}", comment)

	return expandTemplate(ani, t, text, status)
}

// expandTemplate 展开模版中的变量，${notification} 引用全局通知模版
func expandTemplate(ani *entity.Ani, template, text string, status entity.NotificationStatus) string {
	t := expandVariables(ani, template, text, status)
	// 全局模版副本中先剔除 ${notification} 保证递归终止
	if strings.Contains(t, "${notification}") {
		global := strings.ReplaceAll(config.Get().NotificationTemplate, "${notification}", "")
		t = strings.ReplaceAll(t, "${notification}", expandVariables(ani, global, text, status))
	}
	return strings.TrimSpace(t)
}

// expandVariables 替换单个模版中除 ${notification} 外的变量
func expandVariables(ani *entity.Ani, template, text string, status entity.NotificationStatus) string {
	t := strings.ReplaceAll(template, "${text}", text)

	// 从文本中提取集数
	episode := 1.0
	if m := entity.SeasonReg.FindStringSubmatch(text); len(m) > 2 {
		if v, err := strconv.ParseFloat(m[2], 64); err == nil {
			episode = v
		}
	}
	episodeFormat := fmt.Sprintf("%02d", int(episode))
	// x.5
	if episode != float64(int(episode)) {
		episodeFormat += ".5"
	}
	t = strings.ReplaceAll(t, "${episode}", strconv.FormatFloat(episode, 'f', -1, 64))
	t = strings.ReplaceAll(t, "${episodeFormat}", episodeFormat)

	if ani == nil {
		return t
	}

	rd := ani.ReleaseDate
	t = strings.ReplaceAll(t, "${year}", strconv.Itoa(rd.Year()))
	t = strings.ReplaceAll(t, "${month}", strconv.Itoa(int(rd.Month())))
	t = strings.ReplaceAll(t, "${date}", strconv.Itoa(rd.Day()))

	replace := func(placeholder, value string) {
		t = strings.ReplaceAll(t, placeholder, value)
	}
	replace("${title}", ani.Title)
	replace("${score}", fmt.Sprintf("%.1f", ani.Score))
	replace("${season}", strconv.Itoa(ani.Season))
	replace("${seasonFormat}", fmt.Sprintf("%02d", ani.Season))
	replace("${themoviedbName}", ani.ThemoviedbName)
	replace("${bgmUrl}", ani.BgmUrl)
	replace("${currentEpisodeNumber}", strconv.Itoa(ani.CurrentEpisodeNumber))
	replace("${totalEpisodeNumber}", strconv.Itoa(ani.TotalEpisodeNumber))
	replace("${subgroup}", ani.Subgroup)

	tmdbId := ""
	if ani.Tmdb != nil {
		tmdbId = ani.Tmdb.ID
	}
	replace("${tmdbid}", tmdbId)

	tmdbUrl := ""
	if tmdbId != "" {
		// 剧场版为 movie
		tmdbType := "tv"
		if ani.GetOva() {
			tmdbType = "movie"
		}
		tmdbUrl = "https://www.themoviedb.org/" + tmdbType + "/" + tmdbId
	}
	replace("${tmdburl}", tmdbUrl)

	replace("${emoji}", status.Emoji)
	replace("${action}", status.Action)

	replace("${downloadPath}", download.GetDownloadPath(ani))

	// 日文标题与集标题需要查询 BGM 与 TMDB，由 service 注入
	if rename.ReplaceNetworkPlaceholders != nil {
		return rename.ReplaceNetworkPlaceholders(t, ani, episode)
	}
	replace("${jpTitle}", ani.JpTitle)
	defaultEpisodeTitle := "第" + strconv.FormatFloat(episode, 'f', -1, 64) + "集"
	replace("${episodeTitle}", defaultEpisodeTitle)
	replace("${bgmEpisodeTitle}", defaultEpisodeTitle)
	replace("${bgmJpEpisodeTitle}", defaultEpisodeTitle)
	return t
}

// SendTest 测试通知配置：文件移动只校验通知状态，OpenList 上传只校验账户，其他渠道同步发送
func SendTest(n *entity.NotificationConfig, ani *entity.Ani, text string, status entity.NotificationStatus) bool {
	if n.NotificationType != entity.NotifyTypeFileMove && n.NotificationType != entity.NotifyTypeOpenListUpload {
		fn, ok := senders[n.NotificationType]
		return ok && sendOne(fn, n, ani, text, status)
	}
	if !slices.Contains(n.StatusList, entity.StatusDownloadEnd.Name) {
		return false
	}
	if n.NotificationType == entity.NotifyTypeFileMove {
		return true
	}
	if strings.TrimSpace(n.OpenListUploadHost) == "" || n.OpenListUploadApiKey == "" {
		return false
	}
	resp, err := httpc.Get(strings.TrimRight(n.OpenListUploadHost, "/")+"/api/me").
		Set("Authorization", n.OpenListUploadApiKey).
		Do()
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var result struct {
		Code int `json:"code"`
	}
	return resp.AssertStatus() == nil && json.NewDecoder(resp.Body).Decode(&result) == nil && result.Code == 200
}
