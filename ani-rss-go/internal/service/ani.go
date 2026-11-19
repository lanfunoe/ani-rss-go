// Package service — 订阅管理
package service

import (
	"cmp"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"uuid"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/download"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/items"
	"ani-rss-go/internal/store"
	"ani-rss-go/internal/utils"
)

// GetAniFromRss 根据订阅参数构建订阅对象
func GetAniFromRss(ani *entity.Ani) (*entity.Ani, error) {
	if ani == nil {
		return nil, errors.New("参数错误")
	}
	rssURL := ani.URL
	if rssURL == "" {
		return nil, errors.New("RSS地址 不能为空")
	}
	rssType := cmp.Or(ani.Type, "mikan")

	newAni := store.CreateAni()
	newAni.URL = rssURL
	newAni.StandbyRssList = append(newAni.StandbyRssList, ani.StandbyRssList...)

	subgroup := ani.Subgroup
	bgmUrl := ani.BgmUrl

	switch rssType {
	case "mikan":
		if subgroup == "" && bgmUrl == "" {
			subgroupId := items.RssParam(rssURL, "subgroupid")
			if err := MikanFillAni(newAni, subgroupId); err != nil {
				return nil, err
			}
		} else {
			newAni.BgmUrl = bgmUrl
			newAni.Subgroup = subgroup
		}
	case "ani-bt":
		if v := items.RssParam(rssURL, "bgmId"); v != "" {
			newAni.BgmUrl = "https://bgm.tv/subject/" + v
		}
		subgroup = cmp.Or(subgroup, items.RssParam(rssURL, "groupSlug"))
		newAni.Subgroup = subgroup
	case "anime-garden":
		if v := items.RssParam(rssURL, "subject"); v != "" {
			newAni.BgmUrl = "https://bgm.tv/subject/" + v
		}
		if v := items.RssParam(rssURL, "fansub"); v != "" {
			newAni.Subgroup = v
		}
	default:
		newAni.BgmUrl = bgmUrl
	}

	if newAni.BgmUrl == "" {
		return nil, errors.New("bgmUrl 不能为空")
	}

	bgmInfo, err := GetBgmInfo(newAni, true)
	if err != nil {
		return nil, err
	}
	ToAni(bgmInfo, newAni)

	cfg := config.Get()
	// 默认导入全局排除
	if cfg.GetImportExclude() {
		var exclude []string
		for _, e := range slices.Concat(cfg.Exclude, newAni.Exclude) {
			if !slices.Contains(exclude, e) {
				exclude = append(exclude, e)
			}
		}
		newAni.Exclude = exclude
	}

	newAni.DownloadNew = new(cfg.GetDownloadNew())
	newAni.GlobalExclude = cfg.EnabledExclude
	newAni.Type = rssType
	if ani.Enable != nil {
		newAni.Enable = ani.Enable
	}

	newAni.Subgroup = cmp.Or(newAni.Subgroup, entity.SubgroupUnknown)
	if newAni.Subgroup == entity.SubgroupUnknown {
		list, err := items.GetItemsByRssPublic(newAni, rssURL, entity.SubgroupUnknown)
		if err != nil {
			return nil, err
		}
		newAni.Subgroup = items.GetSubgroup(list)
	}

	if newAni.GetOva() {
		return newAni, nil
	}

	// 自动推断剧集偏移
	if cfg.GetOffset() {
		if err := autoOffset(newAni); err != nil {
			return nil, err
		}
	}
	return newAni, nil
}

// AddAni 添加订阅，override 为 true 时覆盖 ID 相同或标题与季相同的订阅
func AddAni(ani *entity.Ani, override bool) error {
	if strings.TrimSpace(ani.Title) == "" {
		return errors.New("标题不能为空")
	}
	if strings.TrimSpace(ani.URL) == "" {
		return errors.New("RSS地址 不能为空")
	}
	if old := store.FindAni(ani.ID); old != nil {
		if !override {
			return errors.New("此订阅已存在")
		}
		store.RemoveAni(old)
	}
	for _, it := range store.ANIList() {
		if it.Title == ani.Title && it.Season == ani.Season {
			if !override {
				return errors.New("订阅标题重复")
			}
			store.RemoveAni(it)
			slog.Info("覆盖订阅", "title", ani.Title, "season", ani.Season)
			break
		}
	}

	store.AddAni(ani)
	store.SyncAniList()

	if ani.GetEnable() {
		go func() {
			if download.Login() {
				DownloadAni(ani)
			}
		}()
	} else {
		go func() {
			list, err := items.GetItems(ani)
			if err != nil {
				slog.Error("获取 RSS 失败", "title", ani.Title, "error", err)
				return
			}
			currentEpisodeNumber := items.CurrentEpisodeNumber(ani, list)
			if tx := store.BeginAni(ani.ID); tx != nil {
				tx.CurrentEpisodeNumber = currentEpisodeNumber
				tx.Commit()
			}
		}()
	}
	slog.Info("添加订阅", "title", ani.Title, "url", ani.URL, "id", ani.ID)
	return nil
}

var setAniMu sync.Mutex

// SetAni 修改订阅
func SetAni(ani *entity.Ani, move bool) error {
	setAniMu.Lock()
	defer setAniMu.Unlock()

	for _, it := range store.ANIList() {
		if it.ID != ani.ID && it.Title == ani.Title && it.Season == ani.Season {
			return errors.New("订阅标题重复")
		}
	}

	tx := store.BeginAni(ani.ID)
	if tx == nil {
		return errors.New("订阅不存在")
	}
	old := *tx.Ani
	*tx.Ani = *ani
	// 保留运行时字段
	tx.CurrentEpisodeNumber, tx.LastDownloadTime = old.CurrentEpisodeNumber, old.LastDownloadTime
	tx.Commit()
	updated := tx.Ani

	if move {
		go func() {
			oldPath := download.GetDownloadPath(&old)
			newPath := download.GetDownloadPath(updated)
			if oldPath == newPath {
				// 位置未发生改变
				return
			}
			for _, info := range download.FindTorrentsInfosByAni(&old) {
				download.SetSavePath(info, newPath)
			}
			os.MkdirAll(newPath, 0o755)
			utils.MoveEntries(oldPath, newPath)
			utils.ClearDir(oldPath)
		}()
	}

	// 种子目录迁移
	oldDir := store.TorrentDir(&old)
	newDir := store.TorrentDir(updated)
	if oldDir != newDir {
		os.MkdirAll(filepath.Dir(newDir), 0o755)
		if _, err := os.Stat(oldDir); err == nil {
			if err := utils.Move(oldDir, newDir); err != nil {
				slog.Error("移动种子目录失败", "from", oldDir, "to", newDir, "error", err)
			}
		}
		utils.ClearDir(oldDir)
	}
	store.SyncAniList()
	slog.Info("修改订阅", "title", ani.Title, "url", ani.URL, "id", ani.ID)
	return nil
}

// DeleteAni 删除订阅
func DeleteAni(ids []string, deleteFiles bool) error {
	if len(ids) == 0 {
		return errors.New("未选择订阅")
	}
	var anis []*entity.Ani
	for _, ani := range store.ANIList() {
		if slices.Contains(ids, ani.ID) {
			anis = append(anis, ani)
		}
	}
	if len(anis) == 0 {
		return errors.New("订阅不存在")
	}
	store.RemoveAni(anis...)
	store.SyncAniList()

	go func() {
		for _, ani := range anis {
			torrentDir := store.TorrentDir(ani)
			os.RemoveAll(torrentDir)
			utils.ClearDir(torrentDir)
			slog.Info("删除订阅", "title", ani.Title, "url", ani.URL, "id", ani.ID)
		}
		if deleteFiles {
			infos := download.FindTorrentsInfosByAni(anis...)
			for _, info := range infos {
				download.DeleteTorrent(info, true, true)
			}
			for _, ani := range anis {
				path := download.GetDownloadPath(ani)
				os.RemoveAll(path)
				slog.Info("删除本地文件", "path", path)
				utils.ClearDir(path)
			}
		}
	}()
	return nil
}

// ListAni 订阅列表（按星期分组）
func ListAni() *entity.ListAni {
	cfg := config.Get()
	out := &entity.ListAni{}

	// 今天排最前
	weeks := slices.Clone(weekLabels[:])
	slices.SortFunc(weeks, func(a, b string) int { return cmp.Compare(weekOrder(a), weekOrder(b)) })
	weekItemsMap := map[string][]*entity.Ani{}
	for _, w := range weeks {
		out.WeekList = append(out.WeekList, entity.WeekAni{WeekLabel: w})
		weekItemsMap[w] = nil
	}

	// 按月份分组
	aniList := store.ANIList()
	months := make([]string, 0, len(aniList))
	for _, ani := range aniList {
		months = append(months, ani.ReleaseDate.Format("2006-01"))
	}
	// 倒序排列后相同月份相邻，Compact 即可去重
	slices.SortFunc(months, func(a, b string) int { return cmp.Compare(b, a) })
	out.ReleaseDateList = slices.Compact(months)
	out.Total = len(aniList)

	// 先计算拼音，供排序与前端检索使用
	sorted := make([]*entity.Ani, 0, len(aniList))
	for _, ani := range aniList {
		view := *ani
		view.Pinyin = utils.GetPinyin(view.Title)
		view.PinyinInitials = utils.GetFirstLetter(view.Title)
		sorted = append(sorted, &view)
	}

	// 排序
	switch cfg.SortType {
	case entity.SortPinyin:
		slices.SortStableFunc(sorted, func(a, b *entity.Ani) int { return cmp.Compare(a.Pinyin, b.Pinyin) })
	case entity.SortDownloadTime:
		slices.SortStableFunc(sorted, func(a, b *entity.Ani) int { return cmp.Compare(b.LastDownloadTime, a.LastDownloadTime) })
	default: // SCORE
		slices.SortStableFunc(sorted, func(a, b *entity.Ani) int { return cmp.Compare(b.Score, a.Score) })
	}

	for i, ani := range sorted {
		ani.Sort = i
		weekIdx := int(ani.ReleaseDate.Weekday())
		weekLabel := weekLabels[weekIdx]
		ani.WeekLabel = weekLabel
		weekItemsMap[weekLabel] = append(weekItemsMap[weekLabel], ani)
	}
	for i := range out.WeekList {
		out.WeekList[i].Items = weekItemsMap[out.WeekList[i].WeekLabel]
		if out.WeekList[i].Items == nil {
			out.WeekList[i].Items = []*entity.Ani{}
		}
	}
	return out
}

// UpdateTotalEpisodeNumber 更新总集数
func UpdateTotalEpisodeNumber(ani *entity.Ani, bgmInfo *entity.BgmInfo, force bool) bool {
	if !force && ani.TotalEpisodeNumber > 0 {
		return false
	}
	eps := GetEps(bgmInfo)
	if eps == ani.TotalEpisodeNumber {
		return false
	}
	tx := store.BeginAni(ani.ID)
	if tx == nil {
		return false
	}
	tx.TotalEpisodeNumber = eps
	tx.Commit()
	slog.Info("总集数发生更新", "title", ani.Title, "eps", eps)
	return true
}

// UpdateTotalEpisodeNumbers 从 BGM 批量更新指定订阅的总集数
func UpdateTotalEpisodeNumbers(ids []string, force bool) {
	slog.Info("开始手动更新总集数")
	count := 0
	for _, ani := range store.ANIList() {
		if !slices.Contains(ids, ani.ID) {
			continue
		}
		info, err := GetBgmInfo(ani, false)
		if err != nil {
			continue
		}
		if UpdateTotalEpisodeNumber(ani, info, force) {
			count++
		}
	}
	store.SyncAniList()
	slog.Info("手动更新总集数完成", "count", count)
}

// BatchEnable 批量启用/停用
func BatchEnable(value bool, ids []string) error {
	if len(ids) == 0 {
		return errors.New("未选择订阅")
	}
	for _, id := range ids {
		if tx := store.BeginAni(id); tx != nil {
			tx.Enable = new(value)
			tx.Commit()
		}
	}
	store.SyncAniList()
	return nil
}

// PreviewAni 预览订阅资源
func PreviewAni(ani *entity.Ani) (map[string]any, error) {
	list, err := items.GetItems(ani)
	if err != nil {
		return nil, err
	}
	downloadPath := download.GetDownloadPath(ani)
	for _, item := range list {
		torrent := store.GetTorrent(ani, item)
		if _, err := os.Stat(torrent); err == nil {
			item.HasDownloaded = new(true)
			item.TorrentPath = torrent
			continue
		}
		item.HasDownloaded = new(ItemDownloaded(ani, item, false))
	}
	return map[string]any{
		"downloadPath": downloadPath,
		"items":        list,
		"omitList":     items.OmitList(ani, list),
	}, nil
}

// DownloadPathPreview 计算下载路径是否改变
func DownloadPathPreview(ani *entity.Ani) map[string]any {
	downloadPath := download.GetDownloadPath(ani)
	change := false
	if old := store.FindAni(ani.ID); old != nil {
		oldCopy := *old
		oldCopy.Season = ani.Season
		change = downloadPath != download.GetDownloadPath(&oldCopy)
	}
	return map[string]any{"change": change, "downloadPath": downloadPath}
}

// ImportAni 导入订阅
func ImportAni(aniList []*entity.Ani, conflict string) error {
	if len(aniList) == 0 {
		return errors.New("导入列表为空")
	}
	for _, ani := range aniList {
		var old *entity.Ani
		for _, it := range store.ANIList() {
			if it.Title == ani.Title && it.Season == ani.Season {
				old = it
				break
			}
		}
		if old == nil {
			ani.Cover = store.SaveCover(ani.Image)
			// 新导入的订阅分配全新 id
			ani.ID = uuid.New().String()
			store.AddAni(ani)
			continue
		}
		if conflict == "SKIP" {
			slog.Info("存在冲突，已跳过", "title", ani.Title, "season", ani.Season)
			continue
		}
		slog.Info("存在冲突，已替换", "title", ani.Title, "season", ani.Season)
		cover := store.SaveCover(ani.Image)
		if tx := store.BeginAni(old.ID); tx != nil {
			id, currentEpisode, lastDownloadTime := tx.ID, tx.CurrentEpisodeNumber, tx.LastDownloadTime
			*tx.Ani = *ani
			tx.Cover, tx.ID = cover, id
			tx.CurrentEpisodeNumber, tx.LastDownloadTime = currentEpisode, lastDownloadTime
			tx.Commit()
		}
	}
	store.SyncAniList()
	return nil
}
