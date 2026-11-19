// Package service — 重命名模板中需要联网获取的占位符
package service

import (
	"cmp"
	"strconv"
	"strings"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/items"
	"ani-rss-go/internal/rename"
	"ani-rss-go/internal/store"
)

func init() {
	// 注入网络占位符替换，避免 rename 反向依赖 service
	rename.ReplaceNetworkPlaceholders = replaceNetworkPlaceholders
}

// replaceNetworkPlaceholders 替换集标题与日文标题占位符，无占位符时不发起网络请求
func replaceNetworkPlaceholders(template string, ani *entity.Ani, episode float64) string {
	if !strings.Contains(template, "${") {
		return template
	}
	if strings.Contains(template, "${jpTitle}") {
		template = strings.ReplaceAll(template, "${jpTitle}", jpTitleOf(ani))
	}

	is5 := items.Is5(episode)
	defaultTitle := "第" + strconv.FormatFloat(episode, 'f', -1, 64) + "集"

	if strings.Contains(template, "${episodeTitle}") {
		title := defaultTitle
		if !is5 {
			title = cmp.Or(TmdbEpisodeTitleMap(ani)[int(episode)], defaultTitle)
		}
		template = strings.ReplaceAll(template, "${episodeTitle}", title)
	}

	if strings.Contains(template, "${bgmEpisodeTitle}") || strings.Contains(template, "${bgmJpEpisodeTitle}") {
		title, jpTitle := defaultTitle, defaultTitle
		if !is5 {
			titles := BgmEpisodeTitleMap(ani)[int(episode)]
			title = cmp.Or(titles.Title, defaultTitle)
			jpTitle = cmp.Or(titles.JpTitle, defaultTitle)
		}
		template = strings.ReplaceAll(template, "${bgmEpisodeTitle}", title)
		template = strings.ReplaceAll(template, "${bgmJpEpisodeTitle}", jpTitle)
	}
	return template
}

// jpTitleOf 获取 bgm 日语标题
func jpTitleOf(ani *entity.Ani) string {
	if ani.JpTitle != "" {
		return ani.JpTitle
	}
	bgmInfo, err := GetBgmInfo(ani, true)
	if err != nil {
		return ""
	}
	if tx := store.BeginAni(ani.ID); tx != nil {
		if tx.JpTitle == "" && tx.BgmUrl == ani.BgmUrl {
			tx.JpTitle = bgmInfo.Name
		}
		tx.Commit()
	}
	return bgmInfo.Name
}
