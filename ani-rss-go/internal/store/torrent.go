package store

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/utils"
)

// TorrentDir 订阅的种子目录：沿用已存在的旧版目录 torrents/<首字母>/<标题>[/Season <季>]，否则为 torrents/<ID 首字符>/<ID>
func TorrentDir(ani *entity.Ani) string {
	cfgDir := config.ConfigDir()
	s := utils.GetPinyinInitialLetters(ani.Title)
	legacy := filepath.Join(cfgDir, "torrents", s, ani.Title, "Season "+strconv.Itoa(ani.Season))
	if ani.GetOva() {
		legacy = filepath.Join(cfgDir, "torrents", s, ani.Title)
	}
	// ID 为空时同样使用旧版目录，避免指向 torrents 根目录
	if _, err := os.Stat(legacy); err == nil || ani.ID == "" {
		return legacy
	}
	return filepath.Join(cfgDir, "torrents", ani.ID[:1], ani.ID)
}

// GetTorrent 种子文件路径，优先使用已保存的 .txt 或 .torrent，否则磁力/ed2k 存 .txt
func GetTorrent(ani *entity.Ani, item *entity.Item) string {
	dir := TorrentDir(ani)
	txtFile := filepath.Join(dir, item.InfoHash+".txt")
	torrentFile := filepath.Join(dir, item.InfoHash+".torrent")
	if _, err := os.Stat(txtFile); err == nil {
		return txtFile
	}
	if _, err := os.Stat(torrentFile); err == nil {
		return torrentFile
	}
	if entity.MagnetReg.MatchString(item.Torrent) || entity.Ed2kReg.MatchString(item.Torrent) {
		return txtFile
	}
	return torrentFile
}

// SaveTorrent 下载并保存种子文件
func SaveTorrent(ani *entity.Ani, item *entity.Item) string {
	file := GetTorrent(ani, item)
	if _, err := os.Stat(file); err == nil {
		return file
	}
	slog.Info("下载种子", "name", item.ReName)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		slog.Error("创建种子目录失败", "error", err)
		return file
	}

	// 磁力与 ed2k 链接直接保存链接文本
	data := []byte(item.Torrent)
	if !entity.MagnetReg.MatchString(item.Torrent) && !entity.Ed2kReg.MatchString(item.Torrent) {
		var err error
		if data, err = fetchTorrent(item.Torrent); err != nil {
			slog.Error("下载种子时出现问题", "error", err)
			return file
		}
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		slog.Error("保存种子失败", "error", err)
		return file
	}
	slog.Info("种子下载完成", "name", item.ReName)
	return file
}

// fetchTorrent 下载种子内容，地址返回 404 时内容为空
func fetchTorrent(torrentURL string) ([]byte, error) {
	resp, err := httpc.Get(torrentURL).Do()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if err := resp.AssertStatus(); err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

// DeleteTorrentFiles 删除订阅种子目录中文件名（不含扩展名）属于 hashList 的种子缓存
func DeleteTorrentFiles(ani *entity.Ani, hashList []string) {
	_ = filepath.WalkDir(TorrentDir(ani), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if slices.Contains(hashList, strings.TrimSuffix(name, filepath.Ext(name))) {
			slog.Info("删除种子", "file", path)
			_ = os.Remove(path)
		}
		return nil
	})
}
