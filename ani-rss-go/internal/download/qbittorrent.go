// qBittorrent 下载器
package download

import (
	"cmp"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/utils"
)

// QBitTorrent qBittorrent WebUI API 实现
type QBitTorrent struct{}

// qbAPI 构造携带鉴权的 qBittorrent WebUI 请求
func qbAPI(method, path string) *httpc.Request {
	cfg := config.Get()
	return httpc.New(method, cfg.DownloadToolHost+path).
		Set("Authorization", "Bearer "+cfg.DownloadToolPassword)
}

// qbPost 发送 POST 请求，状态码 2xx 时返回 true
func qbPost(path string, form map[string]string) bool {
	req := qbAPI(http.MethodPost, path)
	for k, v := range form {
		req.SetForm(k, v)
	}
	resp, err := req.Do()
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.AssertStatus() == nil
}

// Login 校验登录状态
func (q *QBitTorrent) Login(cfg *entity.Config) bool {
	host, password := cfg.DownloadToolHost, cfg.DownloadToolPassword
	if host == "" || password == "" {
		slog.Warn("qBittorrent 未配置完成")
		return false
	}
	if !strings.HasPrefix(password, "qbt_") {
		slog.Warn("qBittorrent 未正确配置 ApiKey")
		return false
	}
	resp, err := httpc.Post(host+"/api/v2/app/version").
		Set("Authorization", "Bearer "+password).
		Do()
	if err != nil {
		slog.Error("登录 qBittorrent 失败", "error", err)
		return false
	}
	defer resp.Body.Close()
	return resp.AssertStatus() == nil
}

// mediaFiles 任务内的视频与字幕文件，大文件排前面
func mediaFiles(hash string) []QbFile {
	list, _ := QbFiles(hash)
	var out []QbFile
	for _, f := range list {
		ext := strings.TrimPrefix(filepath.Ext(f.Name), ".")
		if ext == "" || f.Size < 1 {
			continue
		}
		if utils.IsVideoFormat(ext) || utils.IsSubtitleFormat(ext) {
			out = append(out, f)
		}
	}
	slices.SortStableFunc(out, func(a, b QbFile) int { return cmp.Compare(b.Size, a.Size) })
	return out
}

// addRequest 构造 /torrents/add 请求的公共参数
func addRequest(name, savePath, category string, tags []string) *httpc.Request {
	cfg := config.Get()
	return qbAPI(http.MethodPost, "/api/v2/torrents/add").
		SetForm("addToTopOfQueue", "false").
		SetForm("autoTMM", "false").
		SetForm("category", category).
		SetForm("contentLayout", cfg.QbContentLayout).
		SetForm("dlLimit", strconv.FormatInt(utils.Deref(cfg.DlLimit)*1024, 10)).
		SetForm("firstLastPiecePrio", "false").
		SetForm("rename", name).
		SetForm("savepath", savePath).
		SetForm("sequentialDownload", "false").
		SetForm("skip_checking", "false").
		SetForm("stopCondition", "None").
		SetForm("upLimit", strconv.FormatInt(utils.Deref(cfg.UpLimit)*1024, 10)).
		SetForm("useDownloadPath", strconv.FormatBool(utils.Deref(cfg.QbUseDownloadPath))).
		SetForm("tags", strings.Join(tags, ",")).
		SetForm("ratioLimit", strconv.Itoa(utils.Deref(cfg.RatioLimit))).
		SetForm("seedingTimeLimit", strconv.Itoa(utils.Deref(cfg.SeedingTimeLimit))).
		SetForm("inactiveSeedingTimeLimit", strconv.Itoa(utils.Deref(cfg.InactiveSeedingTimeLimit)))
}

// Download 添加下载任务
func (q *QBitTorrent) Download(ani *entity.Ani, item *entity.Item, savePath string, torrentFile string) bool {
	name := item.ReName
	req := addRequest(name, savePath, entity.TagAniRss, NewTags(ani, item))

	data, err := os.ReadFile(torrentFile)
	if err != nil {
		slog.Error("读取种子文件失败", "error", err)
		return false
	}
	if strings.HasSuffix(torrentFile, ".txt") {
		// 磁力链接文本
		req.SetForm("paused", "false").SetForm("stopped", "false").SetForm("urls", string(data))
	} else if len(data) > 0 {
		// 开启了重命名则先暂停，重命名后再开始
		paused := strconv.FormatBool(config.Get().GetRename())
		req.SetForm("paused", paused).SetForm("stopped", paused).AddFileBytes("torrents", filepath.Base(torrentFile), data)
	} else {
		req.SetForm("paused", "false").SetForm("stopped", "false").
			SetForm("urls", "magnet:?xt=urn:btih:"+strings.TrimSuffix(filepath.Base(torrentFile), ".torrent"))
	}

	resp, err := req.Do()
	if err != nil {
		return false
	}
	resp.Body.Close()

	hash := strings.TrimSuffix(filepath.Base(torrentFile), ".torrent")
	for range 3 {
		time.Sleep(10 * time.Second)
		for _, info := range q.GetTorrentsInfos() {
			if info.Hash == hash || info.Name == name {
				return true
			}
		}
	}
	return false
}

// GetTorrentsInfos 任务列表（过滤 ani-rss 分类或标签），未配置或请求失败时返回空列表
func (q *QBitTorrent) GetTorrentsInfos() []*entity.TorrentsInfo {
	if cfg := config.Get(); cfg.DownloadToolHost == "" || cfg.DownloadToolPassword == "" {
		return nil
	}
	resp, err := qbAPI(http.MethodGet, "/api/v2/torrents/info").Do()
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var list []struct {
		Hash      string  `json:"hash"`
		Name      string  `json:"name"`
		Category  string  `json:"category"`
		Tags      string  `json:"tags"` // 逗号分隔
		State     string  `json:"state"`
		Progress  float64 `json:"progress"`
		Size      int64   `json:"size"`
		Completed int64   `json:"completed"`
		SavePath  string  `json:"save_path"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		slog.Error("解析任务列表失败", "error", err)
		return nil
	}
	var out []*entity.TorrentsInfo
	for _, t := range list {
		var tags []string
		for tag := range strings.SplitSeq(t.Tags, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				tags = append(tags, tag)
			}
		}
		if t.Category != entity.TagAniRss && !slices.Contains(tags, entity.TagAniRss) {
			continue
		}
		// /torrents/info 的 progress 是 0..1 小数，统一按 completed/size 重新计算
		state := cmp.Or(t.State, "downloading")
		// 兼容 < 5.0.0 的旧状态
		switch state {
		case "pausedDL":
			state = "stoppedDL"
		case "pausedUP":
			state = "stoppedUP"
		}
		info := &entity.TorrentsInfo{
			Hash:     t.Hash,
			Name:     t.Name,
			State:    state,
			Category: t.Category,
			TagList:  tags,
			// 统一 Windows 路径分隔符，便于与下载位置比较
			SavePath: utils.AbsPath(t.SavePath),
		}
		setProgress(info, t.Completed, t.Size)
		out = append(out, info)
	}
	return out
}

// Delete 删除任务并清理残留文件夹
func (q *QBitTorrent) Delete(info *entity.TorrentsInfo, deleteFiles bool) bool {
	files, _ := QbFiles(info.Hash)
	if !qbPost("/api/v2/torrents/delete", map[string]string{
		"hashes":      info.Hash,
		"deleteFiles": strconv.FormatBool(deleteFiles),
	}) {
		return false
	}

	// 剧场版不用清理残留文件夹
	if !entity.SeasonReg.MatchString(info.Name) {
		return true
	}

	cfg := config.Get()
	seen := map[string]bool{}
	for _, f := range files {
		dir := filepath.Dir(f.Name)
		if dir == "." || seen[dir] {
			continue
		}
		seen[dir] = true
		full := filepath.Join(info.SavePath, dir)
		if st, err := os.Stat(full); err != nil || !st.IsDir() {
			continue
		}
		if utils.Deref(cfg.SubtitleIndependentFolderEnabled) && filepath.Base(full) == cfg.SubtitleIndependentFolderName {
			continue
		}
		slog.Info("删除剩余文件夹", "dir", full)
		if err := os.RemoveAll(full); err != nil {
			slog.Info("删除失败", "dir", full)
		}
	}
	return true
}

// Rename 重命名任务内文件并开始任务
func (q *QBitTorrent) Rename(info *entity.TorrentsInfo) bool {
	reName := info.Name
	if reName == "" || !entity.SeasonReg.MatchString(reName) {
		// 剧场版 OR OVA 直接开始任务
		if QbStart(info.Hash) {
			slog.Info("开始任务", "name", reName)
			return true
		}
		return false
	}

	ani := FindAni(info)
	if ani == nil {
		slog.Error("未能获取番剧对象", "name", info.Name)
		return false
	}

	files := mediaFiles(info.Hash)
	if len(files) == 0 {
		slog.Debug("磁力链接还在获取原数据中", "hash", info.Hash)
		return false
	}

	if keywords := priorityKeywords(ani); len(keywords) > 0 {
		slices.SortStableFunc(files, func(a, b QbFile) int {
			return cmp.Compare(keywordIndex(keywords, a.Name), keywordIndex(keywords, b.Name))
		})
	}

	cfg := config.Get()
	subtitleFolder := ""
	if utils.Deref(cfg.SubtitleIndependentFolderEnabled) {
		subtitleFolder = cfg.SubtitleIndependentFolderName
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name)
	}

	var newNames []string
	for _, f := range files {
		newPath := GetFileReName(f.Name, reName)
		if subtitleFolder != "" && utils.IsSubtitleFormat(newPath) {
			newPath = subtitleFolder + "/" + newPath
		}
		if slices.Contains(names, newPath) {
			continue
		}
		if slices.Contains(newNames, newPath) {
			// 停止不必要的文件下载
			QbSkipFile(info.Hash, f.Index)
			continue
		}
		newNames = append(newNames, newPath)

		if f.Name == newPath {
			continue
		}
		slog.Info("重命名", "from", f.Name, "to", newPath)
		if !QbRenameFile(info.Hash, f.Name, newPath) {
			return false
		}
	}

	if !QbStart(info.Hash) {
		return false
	}
	slog.Info("开始任务", "name", reName)

	if len(newNames) == 0 {
		return true
	}

	// qb 重命名具有延迟，等待重命名完成
	for range 10 {
		time.Sleep(time.Second)
		current, _ := QbFiles(info.Hash)
		renamed := 0
		for _, f := range current {
			if slices.Contains(newNames, f.Name) {
				renamed++
			}
		}
		if renamed == len(newNames) {
			return true
		}
	}
	return false
}

// AddTags 添加标签
func (q *QBitTorrent) AddTags(info *entity.TorrentsInfo, tags string) bool {
	resp, err := qbAPI(http.MethodPost, "/api/v2/torrents/addTags").
		SetForm("hashes", info.Hash).
		SetForm("tags", tags).
		Do()
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := resp.BodyString()
		slog.Error(body)
		return false
	}
	return true
}

// UpdateTrackers 更新 Trackers
func (q *QBitTorrent) UpdateTrackers(trackers []string) {
	resp, err := qbAPI(http.MethodGet, "/api/v2/app/preferences").Do()
	if err != nil {
		return
	}
	var prefs map[string]any
	err = json.NewDecoder(resp.Body).Decode(&prefs)
	resp.Body.Close()
	if err != nil {
		return
	}
	prefs["add_trackers"] = strings.Join(trackers, "\n")
	prefs["add_trackers_enabled"] = true
	data, _ := json.Marshal(prefs)
	resp2, err := qbAPI(http.MethodPost, "/api/v2/app/setPreferences").
		SetForm("json", string(data)).Do()
	if err != nil {
		return
	}
	defer resp2.Body.Close()
	if resp2.StatusCode < 300 {
		slog.Info("qBittorrent 更新Trackers完成", "count", len(trackers))
	} else {
		slog.Error("qBittorrent 更新Trackers失败", "status", resp2.StatusCode)
	}
}

// SetSavePath 修改保存位置
func (q *QBitTorrent) SetSavePath(info *entity.TorrentsInfo, path string) {
	qbPost("/api/v2/torrents/setAutoManagement", map[string]string{"hashes": info.Hash, "enable": "false"})
	resp, err := qbAPI(http.MethodPost, "/api/v2/torrents/setSavePath").
		SetForm("id", info.Hash).
		SetForm("path", path).Do()
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := resp.BodyString()
		slog.Error(body)
	}
}

func priorityKeywords(ani *entity.Ani) []string {
	if utils.Deref(ani.CustomPriorityKeywordsEnable) {
		return ani.CustomPriorityKeywords
	}
	if cfg := config.Get(); cfg.GetPriorityKeywordsEnable() {
		return cfg.PriorityKeywords
	}
	return nil
}

func keywordIndex(keywords []string, fileName string) int {
	base := filepath.Base(fileName)
	mainName := strings.TrimSuffix(base, filepath.Ext(base))
	if i := slices.IndexFunc(keywords, func(k string) bool { return strings.Contains(mainName, k) }); i >= 0 {
		return i
	}
	return len(keywords)
}

// QbAddPaused 以暂停状态添加种子到 qBittorrent，用于合集下载
func QbAddPaused(name string, torrent []byte, savePath string, tags []string) error {
	resp, err := addRequest(name, savePath, "", tags).
		AddFileBytes("torrents", "collection.torrent", torrent).
		SetForm("paused", "true").
		SetForm("stopped", "true").
		Do()
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return resp.AssertStatus()
}

// QbFile qBittorrent 任务内的文件
type QbFile struct {
	Index    int    `json:"index"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Priority int    `json:"priority"`
}

// QbFiles 获取任务的全部文件
func QbFiles(hash string) ([]QbFile, error) {
	resp, err := qbAPI(http.MethodGet, "/api/v2/torrents/files").AddQuery("hash", hash).Do()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return nil, err
	}
	var list []QbFile
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	return list, nil
}

// QbSkipFile 将文件优先级设为 0（不下载）
func QbSkipFile(hash string, index int) {
	qbPost("/api/v2/torrents/filePrio", map[string]string{"hash": hash, "id": strconv.Itoa(index), "priority": "0"})
}

// QbRenameFile 重命名任务内的文件，状态码 2xx 时返回 true
func QbRenameFile(hash, oldPath, newPath string) bool {
	return qbPost("/api/v2/torrents/renameFile", map[string]string{"hash": hash, "oldPath": oldPath, "newPath": newPath})
}

// QbStart 开始任务
func QbStart(hash string) bool {
	return qbPost("/api/v2/torrents/start", map[string]string{"hashes": hash})
}
