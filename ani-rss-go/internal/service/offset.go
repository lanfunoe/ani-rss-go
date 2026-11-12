// Package service — 自动推断剧集偏移
package service

import (
	"cmp"
	"slices"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/items"
)

// autoOffset 按配置的偏移方式推断主 RSS 与备用 RSS 的剧集偏移，两者共用同一个目标起始集数，
func autoOffset(ani *entity.Ani) error {
	// 目标起始集数在首次遇到非空 RSS 时获取，BGM_OFFSET 需要请求 Bangumi
	minEpisode := 0
	offsetOf := func(rssURL string) (int, error) {
		list, err := items.GetItemsByRssPublic(ani, rssURL, "")
		if err != nil || len(list) == 0 {
			return 0, err
		}
		if minEpisode == 0 {
			minEpisode = targetMinEpisode(ani)
		}
		return episodeOffset(ani, list, minEpisode), nil
	}
	var err error
	if ani.Offset, err = offsetOf(ani.URL); err != nil {
		return err
	}
	for i, rss := range ani.StandbyRssList {
		if ani.StandbyRssList[i].Offset, err = offsetOf(rss.URL); err != nil {
			return err
		}
	}
	return nil
}

// targetMinEpisode 偏移后正篇的起始集数：BGM_OFFSET 取 Bangumi 正篇最小集数，否则为 1
func targetMinEpisode(ani *entity.Ani) int {
	if config.Get().OffsetType == entity.OffsetBgm {
		return bgmMinEpisode(ani)
	}
	return 1
}

// episodeOffset 以 RSS 最小集数为基准对齐到目标起始集数，x.5 集对齐到目标首集的前半集
// 目标首集大于 1 表示跨季连续编号，此时将订阅的季设为 1
func episodeOffset(ani *entity.Ani, list []*entity.Item, minEpisode int) int {
	itemMinEpisode := slices.MinFunc(list, func(a, b *entity.Item) int { return cmp.Compare(a.Episode, b.Episode) }).Episode
	if minEpisode > 1 {
		ani.Season = 1
	}
	if items.Is5(itemMinEpisode) {
		// 例如目标首集为 13 时，将 0.5 或 12.5 对齐到 12.5
		return minEpisode - 1 - int(itemMinEpisode)
	}
	// 整数集直接平移到目标首集，例如 RSS 从 25 开始、目标为 13 时偏移 -12
	return minEpisode - int(itemMinEpisode)
}
