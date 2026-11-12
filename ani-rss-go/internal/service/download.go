// Package service — 下载主流程
package service

import (
	"cmp"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/download"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/items"
	"ani-rss-go/internal/notification"
	"ani-rss-go/internal/store"
	"ani-rss-go/internal/utils"
)

var downloadMu sync.Mutex

func init() {
	// 注入反查订阅，避免 download 反向依赖 service
	download.FindAni = FindAniByDownloadPath
}

// DownloadAni 下载动漫
func DownloadAni(ani *entity.Ani) {
	downloadMu.Lock()
	defer downloadMu.Unlock()

	cfg := config.Get()
	deleteEnabled := cfg.GetDelete()
	autoDisabled := cfg.GetAutoDisabled()
	downloadCount := utils.Deref(cfg.DownloadCount)
	delayedDownload := utils.Deref(cfg.DelayedDownload)
	deleteStandbyOnly := cfg.GetDeleteStandbyRSSOnly()

	title, season := ani.Title, ani.Season
	downloadNew := ani.GetDownloadNew()

	torrentsInfos := download.GetTorrentsInfos()
	currentDownloadCount := 0

	list, err := items.GetItems(ani)
	if err != nil {
		slog.Error("获取 RSS 失败", "title", title, "error", err)
		return
	}
	items.Omit(ani, list)
	slog.Debug("资源统计", "title", title, "count", len(list))

	// 正在下载的任务数
	downloading := 0
	for _, t := range torrentsInfos {
		if !t.Finished() {
			downloading++
		}
	}

	savePath := download.GetDownloadPath(ani)

	synced := false

	for _, item := range list {
		reName := item.ReName
		torrent := store.GetTorrent(ani, item)
		master := item.GetMaster()
		hash := strings.ToLower(strings.TrimSuffix(filepath.Base(torrent), filepath.Ext(torrent)))
		episode := item.Episode
		// 主 RSS 的整数集计入已下载集数
		counted := 0
		if master && !items.Is5(episode) {
			counted = 1
		}

		// 种子记录已存在
		if _, err := os.Stat(torrent); err == nil {
			slog.Debug("种子记录已存在", "name", reName)
			currentDownloadCount += counted
			continue
		}

		// 已被禁止下载
		if slices.Contains(ani.NotDownload, episode) {
			slog.Debug("已被禁止下载", "name", reName)
			currentDownloadCount += counted
			continue
		}

		// 只下载最新集：与最新资源同一天发布，缺少发布时间时仅最新资源
		if downloadNew {
			newItem := list[len(list)-1]
			isNew := item == newItem
			if item.PubDate != nil && newItem.PubDate != nil {
				isNew = item.PubDate.Format(time.DateOnly) == newItem.PubDate.Format(time.DateOnly)
			}
			if !isNew {
				currentDownloadCount += counted
				continue
			}
		}

		// 延迟下载
		if item.PubDate != nil && delayedDownload > 0 {
			threshold := time.Now().Add(-time.Duration(delayedDownload) * time.Minute)
			if threshold.UnixMilli() < item.PubDate.UnixMilli() {
				slog.Info("延迟下载", "name", reName)
				continue
			}
		}

		// 仅在主RSS更新后删除备用RSS
		if deleteEnabled && master && deleteStandbyOnly {
			standby := findStandbyTorrent(torrentsInfos, savePath, reName, ani.Subgroup)
			if standby != nil {
				if !slices.Contains(standby.TagList, entity.TagRename) {
					continue
				}
				if !download.DeleteTorrent(standby, false, false) {
					slog.Debug("备用RSS可能还未做种完成", "name", standby.Name)
					continue
				}
				torrentsInfos = slices.DeleteFunc(torrentsInfos, func(t *entity.TorrentsInfo) bool { return t == standby })
			}
		}

		// hash 已存在任务
		if slices.ContainsFunc(torrentsInfos, func(t *entity.TorrentsInfo) bool { return strings.EqualFold(t.Hash, hash) }) {
			slog.Info("已有下载任务", "hash", hash, "name", reName)
			currentDownloadCount += counted
			continue
		}

		// 本地文件已存在
		if ItemDownloaded(ani, item, true) {
			slog.Info("本地文件已存在", "name", reName)
			currentDownloadCount += counted
			continue
		}

		// 同时下载数量限制
		if downloadCount > 0 && downloading >= downloadCount {
			slog.Debug("达到同时下载数量限制", "limit", downloadCount)
			continue
		}

		// 种子地址返回 404 时保存为空文件，下载器以文件名中的 infoHash 拼接磁力链接下载
		saveTorrent := store.SaveTorrent(ani, item)
		if _, err := os.Stat(saveTorrent); err != nil {
			// 种子下载失败
			continue
		}

		DeleteStandbyRss(ani, item)

		if !store.ContainsAni(ani) {
			return
		}

		synced = true
		downloadTorrent(ani, item, savePath, saveTorrent)

		currentDownloadCount += counted
		downloading++
	}

	if synced {
		ani.CurrentEpisodeNumber = items.CurrentEpisodeNumber(ani, list)
		ani.LastDownloadTime = time.Now().UnixMilli()
		store.SyncAniList()
	}

	items.Procrastinating(ani, list)

	if !autoDisabled || ani.TotalEpisodeNumber < 1 {
		return
	}
	if currentDownloadCount >= ani.TotalEpisodeNumber {
		slog.Info("已全部下载完成, 自动停止订阅", "title", title, "season", season, "total", ani.TotalEpisodeNumber)
		notification.Send(cfg, ani, fmt.Sprintf("%s 订阅已完结", title), entity.StatusCompleted)
		ani.Enable = new(false)
		store.SyncAniList()
	}
}

func findStandbyTorrent(infos []*entity.TorrentsInfo, savePath, reName, subgroup string) *entity.TorrentsInfo {
	m1 := entity.SeasonReg.FindString(reName)
	for _, info := range infos {
		if info.SavePath != savePath {
			continue
		}
		s := entity.SeasonReg.FindString(info.Name)
		if s == "" || s != m1 {
			continue
		}
		if slices.Contains(info.TagList, entity.TagStandbyRss) || !slices.Contains(info.TagList, subgroup) {
			return info
		}
	}
	return nil
}

// DeleteStandbyRss 洗版：删除备用RSS任务与本地文件
func DeleteStandbyRss(ani *entity.Ani, item *entity.Item) {
	cfg := config.Get()
	if !cfg.GetDelete() || !cfg.GetStandbyRss() || cfg.GetCoexist() {
		return
	}
	reName := item.ReName
	episode := entity.SeasonReg.FindString(reName)
	if episode == "" {
		return
	}
	downloadPath := download.GetDownloadPath(ani)
	infos := download.FindTorrentsInfosByAni(ani)
	for _, info := range infos {
		s := entity.SeasonReg.FindString(info.Name)
		if s != "" && strings.EqualFold(s, episode) {
			download.DeleteTorrent(info, true, true)
		}
	}
	entries, _ := os.ReadDir(downloadPath)
	for _, entry := range entries {
		name := entry.Name()
		fn := entity.SeasonReg.FindString(name)
		if fn == "" || !strings.EqualFold(fn, episode) {
			continue
		}
		full := filepath.Join(downloadPath, name)
		if !entry.IsDir() {
			ext := strings.TrimPrefix(filepath.Ext(name), ".")
			if ext == "" {
				continue
			}
			if !utils.IsVideoFormat(ext) && !utils.IsSubtitleFormat(ext) && ext != "nfo" && ext != "bif" && !strings.HasSuffix(name, "-thumb.jpg") {
				continue
			}
		}
		slog.Info("已开启备用RSS, 自动删除", "file", full)
		if err := os.RemoveAll(full); err != nil {
			slog.Error("删除失败", "file", full, "error", err)
		} else {
			slog.Info("删除成功", "file", full)
		}
	}
}

// downloadTorrent 单个资源下载（带重试与通知）
func downloadTorrent(ani *entity.Ani, item *entity.Item, savePath string, torrentFile string) {
	clone := *ani
	clone.Subgroup = cmp.Or(item.Subgroup, entity.SubgroupUnknown)

	name := item.ReName
	slog.Info("添加下载", "name", name)

	time.Sleep(time.Second)

	text := name + " 已更新"
	if !item.GetMaster() {
		text = "(备用RSS) " + text
	}
	notification.Send(config.Get(), &clone, text, entity.StatusDownloadStart)

	for i := range utils.DerefOr(config.Get().DownloadRetry, 3) {
		if download.Get().Download(&clone, item, savePath, torrentFile) {
			return
		}
		slog.Error("下载失败将进行重试", "name", name, "attempt", i+1)
	}

	// 删除下载失败的种子, 下次轮询仍会重试
	os.Remove(torrentFile)
	slog.Error("添加失败，疑似为坏种", "name", name)
	notification.Send(config.Get(), &clone, name+" 添加失败，疑似为坏种", entity.StatusError)
}

// Notification 下载完成通知
func Notification(info *entity.TorrentsInfo) {
	if !info.Finished() {
		return
	}
	if slices.Contains(info.TagList, entity.TagDownloadComplete) {
		return
	}
	if !download.Get().AddTags(info, entity.TagDownloadComplete) {
		return
	}
	ani := FindAniByDownloadPath(info)
	if ani == nil {
		slog.Debug("未能获取番剧对象", "name", info.Name)
		return
	}

	// 根据标签反查字幕组，取第一个匹配的标签
	subgroup := ani.Subgroup
	for _, t := range info.TagList {
		if slices.ContainsFunc(ani.StandbyRssList, func(rss entity.StandbyRss) bool { return rss.Label == t }) {
			subgroup = t
			break
		}
	}
	clone := *ani
	clone.Subgroup = cmp.Or(subgroup, entity.SubgroupUnknown)

	cfg := config.Get()
	if cfg.GetScrape() {
		// 刮削
		Scrape(&clone, false)
	}
	text := info.Name + " 下载完成"
	if slices.Contains(info.TagList, entity.TagStandbyRss) {
		text = "(备用RSS) " + text
	}
	notification.Send(cfg, &clone, text, entity.StatusDownloadEnd)

	// 完结迁移
	CompletedMigration(&clone)
}

// CompletedMigration 番剧完结迁移
func CompletedMigration(ani *entity.Ani) {
	cfg := config.Get()
	if !ani.GetCompletedFlag() || ani.TotalEpisodeNumber < 1 {
		return
	}
	if ani.CurrentEpisodeNumber < ani.TotalEpisodeNumber || ani.GetEnable() || ani.GetOva() {
		return
	}
	if !cfg.GetAutoDisabled() || !utils.Deref(cfg.Completed) {
		return
	}
	oldPath := download.GetDownloadPath(ani)
	completedTemplate := cfg.CompletedPathTemplate
	if utils.Deref(ani.CustomCompleted) {
		completedTemplate = ani.CustomCompletedPathTemplate
	}
	newPath := download.GetDownloadPathByTemplate(ani, completedTemplate)
	infos := download.FindTorrentsInfosByAni(ani)
	if _, err := os.Stat(oldPath); err != nil {
		return
	}
	os.MkdirAll(newPath, 0o755)

	// 下载器移动任务文件后，再迁移旧目录中剩余的文件
	for _, info := range infos {
		download.SetSavePath(info, newPath)
	}
	slog.Info(fmt.Sprintf("订阅已完结 %s, 移动已完结文件", ani.Title))
	utils.MoveEntries(oldPath, newPath)
	// 清理残留文件夹
	utils.ClearDir(oldPath)
}

// ItemDownloaded 判断是否已下载过
func ItemDownloaded(ani *entity.Ani, item *entity.Item, downloadList bool) bool {
	cfg := config.Get()
	if !cfg.GetRename() || cfg.DownloadPathTemplate == "" || !cfg.GetFileExist() {
		return false
	}
	season, ova, reName := ani.Season, ani.GetOva(), item.ReName

	if downloadList {
		infos := download.FindTorrentsInfosByAni(ani)
		for _, info := range infos {
			if strings.EqualFold(info.Name, reName) {
				slog.Info("已存在下载任务", "name", reName)
				store.SaveTorrent(ani, item)
				return true
			}
		}
	}

	downloadPath := download.GetDownloadPath(ani)
	entries, _ := os.ReadDir(downloadPath)
	for _, entry := range entries {
		ext := strings.TrimPrefix(filepath.Ext(entry.Name()), ".")
		if !utils.IsVideoFormat(ext) {
			continue
		}
		if ova {
			store.SaveTorrent(ani, item)
			slog.Info("本地已存在", "name", reName)
			return true
		}
		m := entity.SeasonReg.FindStringSubmatch(entry.Name())
		if len(m) < 3 {
			continue
		}
		s, err1 := strconv.Atoi(m[1])
		e, err2 := strconv.ParseFloat(m[2], 64)
		if err1 == nil && err2 == nil && s == season && e == item.Episode {
			// 保存 torrent 下次只校验 torrent 是否存在
			store.SaveTorrent(ani, item)
			slog.Info("本地已存在", "name", reName)
			return true
		}
	}
	return false
}

// FindAniByDownloadPath 根据任务反查订阅
func FindAniByDownloadPath(info *entity.TorrentsInfo) *entity.Ani {
	for _, ani := range store.ANIList() {
		if download.GetDownloadPath(ani) == info.SavePath {
			clone := *ani
			return &clone
		}
	}
	return nil
}
