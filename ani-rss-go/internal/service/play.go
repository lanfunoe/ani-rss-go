// Package service — 在线播放：视频列表与媒体文件访问
package service

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"ani-rss-go/internal/download"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/store"
	"ani-rss-go/internal/utils"
)

// PlayItem 视频
type PlayItem struct {
	// 显示标题
	Title string `json:"title"`
	// 路径+文件名
	Filename   string  `json:"filename"`
	Name       string  `json:"name"`
	LastModify int64   `json:"lastModify"`
	Episode    float64 `json:"episode"`
	FormatSize string  `json:"formatSize"`
	ExtName    string  `json:"extName"`
}

// PlayList 获取订阅下载目录中的视频列表，按集数排序
func PlayList(rssURL string) ([]PlayItem, error) {
	var ani *entity.Ani
	for _, it := range store.ANIList() {
		if it.URL == rssURL {
			ani = it
			break
		}
	}
	if ani == nil {
		return nil, errors.New("订阅不存在")
	}
	items := []PlayItem{}
	seen := map[string]bool{}
	_ = filepath.WalkDir(download.GetDownloadPath(ani), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !utils.IsVideoFormat(d.Name()) {
			return nil
		}
		info, err := d.Info()
		// 视频文件小于 20MB 跳过
		if err != nil || info.Size() < 20<<20 {
			return nil
		}
		item := PlayItem{
			Title:      d.Name(),
			Filename:   filepath.ToSlash(path),
			Name:       d.Name(),
			LastModify: info.ModTime().UnixMilli(),
			Episode:    1,
			FormatSize: utils.FormatSize(info.Size(), true),
			ExtName:    strings.TrimPrefix(filepath.Ext(d.Name()), "."),
		}
		// 如匹配正则则用正则取 S01E01
		if m := entity.SeasonReg.FindStringSubmatch(d.Name()); m != nil {
			item.Title = m[0]
			item.Episode, _ = strconv.ParseFloat(m[2], 64)
		}
		// 去重复
		if seen[item.Title] {
			return nil
		}
		seen[item.Title] = true
		items = append(items, item)
		return nil
	})
	slices.SortStableFunc(items, func(a, b PlayItem) int {
		return cmp.Compare(a.Episode, b.Episode)
	})
	return items, nil
}

// OpenMediaFile 打开位于任一订阅下载目录内的视频或字幕文件，拒绝目录越界与越界符号链接
func OpenMediaFile(filename string) (*os.File, error) {
	if !utils.IsVideoFormat(filename) && !utils.IsSubtitleFormat(filename) {
		return nil, os.ErrPermission
	}
	filename = filepath.Clean(filename)
	for _, ani := range store.ANIList() {
		dir := filepath.Clean(download.GetDownloadPath(ani))
		rel, err := filepath.Rel(dir, filename)
		if err != nil || !filepath.IsLocal(rel) {
			continue
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			continue
		}
		file, err := root.Open(rel)
		root.Close()
		if err != nil {
			return nil, err
		}
		return file, nil
	}
	return nil, os.ErrPermission
}
