// Package service — 合集下载
package service

import (
	"cmp"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"ani-rss-go/internal/bencode"
	"ani-rss-go/internal/config"
	"ani-rss-go/internal/download"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/items"
	"ani-rss-go/internal/rename"
	"ani-rss-go/internal/utils"
)

// CollectionInfo 合集信息
type CollectionInfo struct {
	// 种子文件 base64
	Torrent string          `json:"torrent"`
	Ani     *entity.Ani     `json:"ani"`
	BgmInfo *entity.BgmInfo `json:"bgmInfo"`
}

var trailingSlashReg = regexp.MustCompile(`[\\/]$`)

// collectionPreview 解析合集种子并计算每个文件的重命名结果
func collectionPreview(info *CollectionInfo) ([]*entity.Item, []byte, error) {
	if info.Ani == nil {
		return nil, nil, errors.New("订阅为空")
	}
	data, err := base64.StdEncoding.DecodeString(info.Torrent)
	if err != nil {
		return nil, nil, fmt.Errorf("种子解析失败: %w", err)
	}
	files, err := bencode.Files(data)
	if err != nil {
		return nil, nil, fmt.Errorf("种子解析失败: %w", err)
	}

	ani := info.Ani
	// {{字幕组}}:规则 仅对相同字幕组生效
	ruleOf := func(s string) string {
		m := entity.SubgroupRe.FindStringSubmatch(s)
		if m == nil {
			return s
		}
		if m[1] == ani.Subgroup {
			return m[2]
		}
		return ""
	}
	anyMatch := func(rules []string, name string, want bool) bool {
		for _, r := range rules {
			rule := ruleOf(r)
			if rule == "" {
				continue
			}
			re, err := regexp.Compile(rule)
			if err != nil {
				continue
			}
			if re.MatchString(name) == want {
				return true
			}
		}
		return false
	}

	globalExclude := config.Get().Exclude
	list := []*entity.Item{}
	for _, f := range files {
		name := strings.ReplaceAll(trailingSlashReg.ReplaceAllString(f.Path, ""), `\`, "/")
		if strings.HasPrefix(name, "_____padding_file_") && strings.Contains(name, "BitComet") {
			continue
		}
		// 排除
		if anyMatch(ani.Exclude, name, true) {
			continue
		}
		// 匹配
		if anyMatch(ani.Match, name, false) {
			continue
		}
		// 全局排除
		if ani.GetGlobalExclude() && anyMatch(globalExclude, name, true) {
			continue
		}

		item := &entity.Item{
			Title:      name,
			Length:     f.Length,
			FormatSize: utils.FormatSize(f.Length, true),
			Subgroup:   ani.Subgroup,
		}
		rename.Rename(ani, item)
		if strings.TrimSpace(item.ReName) == "" {
			continue
		}
		ext := strings.TrimPrefix(path.Ext(name), ".")
		reName := item.ReName
		if utils.IsSubtitleFormat(ext) {
			if lang := strings.TrimPrefix(path.Ext(strings.TrimSuffix(name, path.Ext(name))), "."); lang != "" {
				reName += "." + lang
			}
		}
		item.ReName = reName + "." + ext
		list = append(list, item)
	}
	return list, data, nil
}

// PreviewCollection 合集预览，按集数排序
func PreviewCollection(info *CollectionInfo) ([]*entity.Item, error) {
	list, _, err := collectionPreview(info)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(list, func(a, b *entity.Item) int {
		return cmp.Compare(a.Episode, b.Episode)
	})
	return list, nil
}

// CollectionSubgroup 获取合集字幕组
func CollectionSubgroup(info *CollectionInfo) (string, error) {
	list, err := PreviewCollection(info)
	if err != nil {
		return "", err
	}
	return items.GetSubgroup(list), nil
}

// StartCollection 开始下载合集：暂停添加 → 重命名所需文件、跳过其余文件 → 开始
func StartCollection(info *CollectionInfo) error {
	cfg := config.Get()
	if cfg.DownloadToolType != "qBittorrent" {
		return errors.New("合集下载暂时只支持 qBittorrent")
	}
	if !download.Login() {
		return errors.New("下载器登录失败")
	}
	preview, data, err := collectionPreview(info)
	if err != nil {
		return err
	}
	hash, err := bencode.InfoHash(data)
	if err != nil {
		return fmt.Errorf("种子解析失败: %w", err)
	}

	ani := info.Ani
	name := fmt.Sprintf("[%s] %s 第%d季", ani.Subgroup, ani.Title, ani.Season)
	if err := download.QbAddPaused(name, data, ani.CustomDownloadPathTemplate, []string{"ANI-RSS合集下载", ani.Subgroup}); err != nil {
		return err
	}

	var files []download.QbFile
	for range 5 {
		time.Sleep(500 * time.Millisecond)
		if files, err = download.QbFiles(hash); err != nil {
			slog.Error(err.Error())
		}
		if len(files) > 0 {
			// 添加下载完成
			break
		}
	}

	// qBittorrent 中的原路径 → 新路径
	reNameMap := map[string]string{}
	newPaths := map[string]bool{}
	for _, item := range preview {
		for _, f := range files {
			if path.Base(f.Name) == path.Base(item.Title) && f.Size == item.Length {
				reNameMap[f.Name] = item.ReName
				newPaths[item.ReName] = true
				break
			}
		}
	}

	for range 30 {
		for _, f := range files {
			newPath, exists := reNameMap[f.Name]
			if !exists {
				if !newPaths[f.Name] && f.Priority > 0 {
					download.QbSkipFile(hash, f.Index)
				}
				continue
			}
			slog.Info(fmt.Sprintf("重命名 %s ==> %s", f.Name, newPath))
			download.QbRenameFile(hash, f.Name, newPath)
		}
		files, _ = download.QbFiles(hash)
		current := map[string]bool{}
		for _, f := range files {
			current[f.Name] = true
		}
		done := true
		for newPath := range newPaths {
			if !current[newPath] {
				done = false
				break
			}
		}
		if done {
			// 所有命名已完成
			break
		}
		// 命名有遗漏 继续
		time.Sleep(time.Second)
	}

	download.QbStart(hash)
	return nil
}
