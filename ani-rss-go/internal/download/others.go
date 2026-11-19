// Transmission 与 Aria2 下载器
package download

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/utils"
)

// rpcResponse JSON-RPC 2.0 响应
type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Transmission RPC 实现
type Transmission struct {
	session atomic.Pointer[string]
}

// rpc 发送 JSON-RPC 2.0 请求，result 非 nil 时接收响应的 result 字段
func (t *Transmission) rpc(cfg *entity.Config, method string, params, result any) error {
	if params == nil {
		params = map[string]any{}
	}
	data, _ := json.Marshal(map[string]any{
		"id":      cfg.Uuid,
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	resp, err := t.post(cfg, data)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusConflict {
		// 409 表示需要携带 session id 重试
		resp.Body.Close()
		if resp, err = t.post(cfg, data); err != nil {
			return err
		}
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return err
	}
	var out rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if out.Error != nil {
		return fmt.Errorf("Transmission %s: %s", method, out.Error.Message)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(out.Result, result)
}

// post 携带 session id 与鉴权发送 RPC 请求，并记录响应中的 session id
func (t *Transmission) post(cfg *entity.Config, data []byte) (*httpc.Response, error) {
	req := httpc.Post(strings.TrimRight(cfg.DownloadToolHost, "/")+"/transmission/rpc").
		Set("Content-Type", "application/json").
		SetBody(data)
	if sid := t.session.Load(); sid != nil {
		req.Set("X-Transmission-Session-Id", *sid)
	}
	if cfg.DownloadToolUsername != "" {
		req.Set("Authorization", basicAuth(cfg.DownloadToolUsername, cfg.DownloadToolPassword))
	}
	resp, err := req.Do()
	if err != nil {
		return nil, err
	}
	if sid := resp.Header.Get("X-Transmission-Session-Id"); sid != "" {
		t.session.Store(&sid)
	}
	return resp, nil
}

// basicAuth HTTP Basic 鉴权头
func basicAuth(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func (t *Transmission) Login(cfg *entity.Config) bool {
	if cfg.DownloadToolHost == "" {
		slog.Warn("Transmission 未配置完成")
		return false
	}
	if err := t.rpc(cfg, "session_get", nil, nil); err != nil {
		slog.Error("登录 Transmission 失败", "error", err)
		return false
	}
	return true
}

func (t *Transmission) GetTorrentsInfos() []*entity.TorrentsInfo {
	cfg := config.Get()
	var result struct {
		Torrents []struct {
			ID          json.Number `json:"id"`
			HashString  string      `json:"hash_string"`
			Name        string      `json:"name"`
			Labels      []string    `json:"labels"`
			IsFinished  bool        `json:"is_finished"`
			Status      int         `json:"status"`
			TotalSize   int64       `json:"total_size"`
			HaveValid   int64       `json:"have_valid"`
			DownloadDir string      `json:"download_dir"`
		} `json:"torrents"`
	}
	err := t.rpc(cfg, "torrent_get", map[string]any{
		"fields": []string{
			"name", "labels", "hash_string", "files",
			"is_finished", "id", "download_dir",
			"status", "total_size", "have_valid",
		},
	}, &result)
	if err != nil {
		return nil
	}
	var list []*entity.TorrentsInfo
	for _, tr := range result.Torrents {
		if !slices.Contains(tr.Labels, entity.TagAniRss) {
			continue
		}
		state := "downloading"
		if tr.Status == 6 {
			// 做种中
			state = "stalledUP"
		} else if tr.IsFinished {
			// 已完成
			state = "stoppedUP"
		}
		info := &entity.TorrentsInfo{
			ID:       tr.ID.String(),
			Hash:     tr.HashString,
			Name:     tr.Name,
			State:    state,
			TagList:  tr.Labels,
			SavePath: utils.AbsPath(tr.DownloadDir),
		}
		setProgress(info, tr.HaveValid, tr.TotalSize)
		list = append(list, info)
	}
	return list
}

// torrentAddArguments 构造 torrent_add 参数
// 磁力链接放 filename，种子内容放 metainfo
func (t *Transmission) torrentAddArguments(ani *entity.Ani, item *entity.Item, savePath string, data []byte, baseName string) map[string]any {
	args := map[string]any{
		"labels":       NewTags(ani, item),
		"download_dir": savePath,
		"metainfo":     "",
		"filename":     "",
		"paused":       false,
	}
	ext := strings.TrimPrefix(filepath.Ext(baseName), ".")
	switch {
	case ext == "txt":
		// 磁力链接文本
		args["filename"] = string(data)
	case len(data) > 0:
		args["metainfo"] = base64.StdEncoding.EncodeToString(data)
	default:
		// 空文件以文件名作为 infoHash
		args["filename"] = "magnet:?xt=urn:btih:" + strings.TrimSuffix(baseName, filepath.Ext(baseName))
	}
	return args
}

func (t *Transmission) Download(ani *entity.Ani, item *entity.Item, savePath string, torrentFile string) bool {
	cfg := config.Get()
	name := item.ReName

	data, err := os.ReadFile(torrentFile)
	if err != nil {
		return false
	}
	args := t.torrentAddArguments(ani, item, savePath, data, filepath.Base(torrentFile))

	var result struct {
		TorrentAdded struct {
			HashString string `json:"hash_string"`
		} `json:"torrent_added"`
	}
	if err := t.rpc(cfg, "torrent_add", args, &result); err != nil {
		return false
	}
	hash := result.TorrentAdded.HashString
	if hash == "" {
		return false
	}

	slog.Info("tr 添加下载", "name", name, "hash", hash)

	if !ani.GetOva() {
		putRenameCache(hash, name)
	}

	for range 3 {
		time.Sleep(10 * time.Second)
		for _, info := range t.GetTorrentsInfos() {
			if info.Hash == hash {
				return true
			}
		}
	}
	return false
}

func (t *Transmission) Delete(info *entity.TorrentsInfo, deleteFiles bool) bool {
	cfg := config.Get()
	return t.rpc(cfg, "torrent_remove", map[string]any{
		"ids":               []string{info.Hash},
		"delete_local_data": deleteFiles,
	}, nil) == nil
}

// 磁力链接元数据名
var magnetMetadataReg = regexp.MustCompile(`^\w{40}$`)

func (t *Transmission) Rename(info *entity.TorrentsInfo) bool {
	cfg := config.Get()
	hash := info.Hash
	name := info.Name

	if magnetMetadataReg.MatchString(name) {
		slog.Debug("磁力链接还在获取原数据中", "name", name)
		return false
	}

	reName := getRenameCache(hash)
	if reName == "" {
		slog.Debug("未获取到重命名", "hash", hash)
		return false
	}

	if ext := filepath.Ext(name); ext != "" {
		reName += ext
	}

	slog.Info("重命名", "from", name, "to", reName)

	err := t.rpc(cfg, "torrent_rename_path", map[string]any{
		"ids":  []string{hash},
		"path": name,
		"name": reName,
	}, nil)
	if err != nil {
		return false
	}
	removeRenameCache(hash)

	for range 10 {
		time.Sleep(time.Second)
		infos := t.GetTorrentsInfos()
		i := slices.IndexFunc(infos, func(cur *entity.TorrentsInfo) bool { return cur.Hash == hash })
		if i < 0 {
			break
		}
		if infos[i].Name == reName {
			return true
		}
	}

	slog.Warn("重命名貌似出现了问题？", "name", reName)
	return false
}

func (t *Transmission) AddTags(info *entity.TorrentsInfo, tags string) bool {
	cfg := config.Get()
	tagList := slices.Concat(info.TagList, strings.Split(tags, ","))
	return t.rpc(cfg, "torrent_set", map[string]any{
		"ids":    []string{info.Hash},
		"labels": tagList,
	}, nil) == nil
}

func (t *Transmission) UpdateTrackers(trackers []string) {
	slog.Info("Transmission暂时还不支持 自动更新Trackers")
}

func (t *Transmission) SetSavePath(info *entity.TorrentsInfo, path string) {
	cfg := config.Get()
	t.rpc(cfg, "torrent_set_location", map[string]any{
		"ids":      []string{info.Hash},
		"location": path,
	}, nil)
}

// Aria2 JSON-RPC 实现
type Aria2 struct{}

// rpc 发送 JSON-RPC 2.0 请求，result 非 nil 时接收响应的 result 字段
func (a *Aria2) rpc(cfg *entity.Config, method string, params []any, result any) error {
	secret := cfg.DownloadToolPassword
	if secret != "" {
		params = append([]any{"token:" + secret}, params...)
	}
	data, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "ani-rss",
		"method":  "aria2." + method,
		"params":  params,
	})
	req := httpc.Post(strings.TrimRight(cfg.DownloadToolHost, "/")+"/jsonrpc").
		Set("Content-Type", "application/json").
		SetBody(data)
	if cfg.DownloadToolUsername != "" {
		req.Set("Authorization", basicAuth(cfg.DownloadToolUsername, secret))
	}
	resp, err := req.Do()
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if out.Error != nil {
		return errors.New(out.Error.Message)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(out.Result, result)
}

func (a *Aria2) Login(cfg *entity.Config) bool {
	if cfg.DownloadToolHost == "" {
		slog.Warn("Aria2 未配置完成")
		return false
	}
	if err := a.rpc(cfg, "getGlobalStat", []any{}, nil); err != nil {
		slog.Error("登录 Aria2 失败", "error", err)
		return false
	}
	return true
}

// aria2Torrent 任务信息，长度字段在响应中是字符串
type aria2Torrent struct {
	GID             string `json:"gid"`
	Status          string `json:"status"`
	TotalLength     int64  `json:"totalLength,string"`
	CompletedLength int64  `json:"completedLength,string"`
	InfoHash        string `json:"infoHash"`
	Dir             string `json:"dir"`
	Bittorrent      *struct {
		Info *struct {
			Name string `json:"name"`
		} `json:"info"`
	} `json:"bittorrent"`
	Files []struct {
		Path string `json:"path"`
	} `json:"files"`
}

// torrents 查询任务原始数据
func (a *Aria2) torrents(cfg *entity.Config) []aria2Torrent {
	fields := []string{"gid", "totalLength", "completedLength", "status", "files", "bittorrent", "infoHash", "dir"}
	var out []aria2Torrent
	for _, method := range []string{"tellActive", "tellWaiting", "tellStopped"} {
		params := []any{}
		if method != "tellActive" {
			params = append(params, -1, 1000)
		}
		params = append(params, fields)
		var list []aria2Torrent
		if err := a.rpc(cfg, method, params, &list); err != nil {
			continue
		}
		out = append(out, list...)
	}
	return out
}

func (a *Aria2) GetTorrentsInfos() []*entity.TorrentsInfo {
	cfg := config.Get()
	time.Sleep(time.Second)
	var out []*entity.TorrentsInfo
	for _, st := range a.torrents(cfg) {
		// 仅保留 BT 任务
		if st.Bittorrent == nil || st.Bittorrent.Info == nil || st.Bittorrent.Info.Name == "" {
			continue
		}
		// 仅下载完成（complete）后才会触发重命名
		state := "downloading"
		if st.Status == "complete" {
			state = "stoppedUP"
		}
		info := &entity.TorrentsInfo{
			ID:       st.GID,
			Hash:     st.InfoHash,
			Name:     st.Bittorrent.Info.Name,
			State:    state,
			TagList:  []string{},
			SavePath: utils.AbsPath(st.Dir),
		}
		setProgress(info, st.CompletedLength, st.TotalLength)
		out = append(out, info)
	}
	return out
}

func (a *Aria2) Download(ani *entity.Ani, item *entity.Item, savePath string, torrentFile string) bool {
	cfg := config.Get()
	name := item.ReName

	ext := strings.TrimPrefix(filepath.Ext(filepath.Base(torrentFile)), ".")
	if ext == "" {
		return false
	}
	if ext == "txt" {
		slog.Error("Aria2 暂不支持磁力链接下载与重命名")
		return false
	}
	data, err := os.ReadFile(torrentFile)
	if err != nil {
		return false
	}
	torrent := base64.StdEncoding.EncodeToString(data)
	var id string
	err = a.rpc(cfg, "addTorrent", []any{torrent, []any{}, map[string]any{"dir": savePath}}, &id)
	if err != nil || id == "" {
		return false
	}

	slog.Info("aria2 添加下载", "name", name, "id", id)

	if !ani.GetOva() {
		putRenameCache(id, name)
	}

	for range 3 {
		time.Sleep(10 * time.Second)
		for _, info := range a.GetTorrentsInfos() {
			if info.ID == id {
				return true
			}
		}
	}
	return false
}

func (a *Aria2) Delete(info *entity.TorrentsInfo, deleteFiles bool) bool {
	cfg := config.Get()
	return a.rpc(cfg, "removeDownloadResult", []any{info.ID}, nil) == nil
}

func (a *Aria2) Rename(info *entity.TorrentsInfo) bool {
	cfg := config.Get()
	id := info.ID

	// 仅支持下载完成后重命名
	if info.State != "stoppedUP" {
		return false
	}

	reName := getRenameCache(id)
	if reName == "" {
		slog.Debug("未获取到重命名", "id", id)
		return false
	}

	// 重新查询任务，取文件列表
	var paths []string
	for _, st := range a.torrents(cfg) {
		if st.GID == id {
			for _, f := range st.Files {
				paths = append(paths, f.Path)
			}
			break
		}
	}

	type target struct {
		path string
		size int64
	}
	var files []target
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() || st.Size() < 1 {
			continue
		}
		ext := strings.TrimPrefix(filepath.Ext(p), ".")
		if ext == "" {
			continue
		}
		if !utils.IsVideoFormat(ext) && !utils.IsSubtitleFormat(ext) {
			continue
		}
		files = append(files, target{path: p, size: st.Size()})
	}
	// 大文件排前面
	slices.SortStableFunc(files, func(a, b target) int { return cmp.Compare(b.size, a.size) })
	if len(files) == 0 {
		slog.Error("映射路径存在错误, 无法重命名")
		return false
	}

	for _, src := range files {
		name := filepath.Base(src.path)
		fileReName := GetFileReName(name, reName)
		newPath := filepath.Join(info.SavePath, fileReName)
		if src.path == newPath {
			continue
		}
		if _, err := os.Lstat(newPath); err == nil {
			os.Remove(newPath)
		}
		if err := os.Rename(src.path, newPath); err != nil {
			return false
		}
		slog.Info("重命名", "from", name, "to", newPath)
	}
	removeRenameCache(id)

	return true
}

func (a *Aria2) AddTags(info *entity.TorrentsInfo, tags string) bool {
	// Aria2 不支持标签
	return false
}

func (a *Aria2) UpdateTrackers(trackers []string) {
	cfg := config.Get()
	err := a.rpc(cfg, "changeGlobalOption", []any{map[string]any{
		"bt-tracker": strings.Join(trackers, ", "),
	}}, nil)
	if err != nil {
		slog.Error("Aria2 更新Trackers失败", "error", err)
		return
	}
	slog.Info("Aria2 更新Trackers完成", "count", len(trackers))
}

func (a *Aria2) SetSavePath(info *entity.TorrentsInfo, path string) {
	// api 不支持
}
