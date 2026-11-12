// Package service — AnimeGarden 番剧浏览
package service

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/utils"
)

const animeGardenHost = "https://api.animes.garden"

// AnimeGardenWeek AnimeGarden 星期分组
type AnimeGardenWeek struct {
	WeekLabel string               `json:"weekLabel"`
	Subjects  []AnimeGardenSubject `json:"subjects"`
}

type AnimeGardenSubject struct {
	ID         entity.FlexString `json:"id"`
	Name       string            `json:"name"`
	Keywords   []string          `json:"keywords"`
	ActivedAt  entity.DateTime   `json:"activedAt"`
	IsArchived entity.FlexString `json:"isArchived"`
	WeekLabel  string            `json:"weekLabel"`
	Exists     bool              `json:"exists"`
	Score      float64           `json:"score"`
	Cover      string            `json:"cover"`
}

type AnimeGardenGroup struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	LastUpdatedAt entity.DateTime   `json:"lastUpdatedAt"`
	Items         []AnimeGardenItem `json:"items"`
	Rss           string            `json:"rss"`
	BgmId         string            `json:"bgmId"`
	GroupRegex    entity.GroupRegex `json:"groupRegex"`
}

type AnimeGardenItem struct {
	ID         entity.FlexString     `json:"id"`
	Provider   string                `json:"provider"`
	ProviderId entity.FlexString     `json:"providerId"`
	Title      string                `json:"title"`
	Href       string                `json:"href"`
	Type       string                `json:"type"`
	Magnet     string                `json:"magnet"`
	Size       int64                 `json:"size"`
	FormatSize string                `json:"formatSize"`
	CreatedAt  entity.DateTime       `json:"createdAt"`
	FetchedAt  entity.DateTime       `json:"fetchedAt"`
	SubjectId  entity.FlexString     `json:"subjectId"`
	Publisher  *AnimeGardenPublisher `json:"publisher"`
	Fansub     *AnimeGardenFansub    `json:"fansub"`
}

type AnimeGardenPublisher struct {
	ID     entity.FlexString `json:"id"`
	Name   string            `json:"name"`
	Avatar string            `json:"avatar"`
}

type AnimeGardenFansub struct {
	ID   entity.FlexString `json:"id"`
	Name string            `json:"name"`
}

// AnimeGardenList 获取 AnimeGarden 番剧列表，指定 bgmUrl 时只返回该番剧
func AnimeGardenList(bgmUrl string) ([]AnimeGardenWeek, error) {
	if bgmUrl != "" {
		bgmId := entity.SubjectIdFromUrl(bgmUrl)
		info := bgmInfoBySubjectId(bgmId, false)
		if info == nil {
			return nil, errors.New("获取 bgmInfo 失败!")
		}
		subject := AnimeGardenSubject{
			ID:     entity.FlexString(bgmId),
			Name:   GetFinalName(info),
			Cover:  info.Image("small"),
			Exists: true,
		}
		return []AnimeGardenWeek{{WeekLabel: "搜索", Subjects: []AnimeGardenSubject{subject}}}, nil
	}

	var scores map[string]float64
	var wg sync.WaitGroup
	wg.Go(func() { scores = donateScore("bgm") })

	out := struct {
		Subjects []AnimeGardenSubject `json:"subjects"`
	}{}
	err := animeGardenGet("/subjects", &out)
	wg.Wait()
	if err != nil {
		return nil, err
	}

	bgmIdSet := subscribedBgmIds()
	weekMap := map[string][]AnimeGardenSubject{}
	slices.SortStableFunc(out.Subjects, func(a, b AnimeGardenSubject) int {
		return cmp.Compare(scores[string(b.ID)], scores[string(a.ID)])
	})
	for _, subject := range out.Subjects {
		id := string(subject.ID)
		subject.Score = scores[id]
		subject.Cover = "https://bgm.animes.garden/bangumi/subject/" + id + "/poster.jpeg?quality=small"
		subject.Exists = bgmIdSet[id]
		subject.WeekLabel = weekLabels[subject.ActivedAt.Weekday()]
		weekMap[subject.WeekLabel] = append(weekMap[subject.WeekLabel], subject)
	}

	weekList := []AnimeGardenWeek{}
	for _, label := range weekLabels {
		if subjects, exists := weekMap[label]; exists {
			weekList = append(weekList, AnimeGardenWeek{WeekLabel: label, Subjects: subjects})
		}
	}
	slices.SortStableFunc(weekList, func(a, b AnimeGardenWeek) int {
		return cmp.Compare(weekOrder(a.WeekLabel), weekOrder(b.WeekLabel))
	})
	return weekList, nil
}

// AnimeGardenGroups 获取番剧的字幕组列表，按最近更新排序
func AnimeGardenGroups(bgmId string) ([]AnimeGardenGroup, error) {
	params := url.Values{}
	params.Set("subject", bgmId)
	params.Set("pageSize", "200")
	params.Set("duplicate", "false")
	out := struct {
		Resources []AnimeGardenItem `json:"resources"`
	}{}
	if err := animeGardenGet("/resources?"+params.Encode(), &out); err != nil {
		return nil, err
	}

	groupItems := map[string][]AnimeGardenItem{}
	groups := []AnimeGardenGroup{}
	for _, item := range out.Resources {
		if item.Fansub == nil {
			continue
		}
		item.FormatSize = utils.FormatSize(item.Size, true)
		id := string(item.Fansub.ID)
		groupItems[id] = append(groupItems[id], item)
		groups = append(groups, AnimeGardenGroup{
			ID:            id,
			Name:          item.Fansub.Name,
			LastUpdatedAt: item.CreatedAt,
			Rss:           fmt.Sprintf("%s/feed.xml?subject=%s&fansub=%s", animeGardenHost, bgmId, strings.ReplaceAll(item.Fansub.Name, "&", "%26")),
			BgmId:         bgmId,
		})
	}
	slices.SortStableFunc(groups, func(a, b AnimeGardenGroup) int {
		return b.LastUpdatedAt.Compare(a.LastUpdatedAt.Time)
	})

	seen := map[string]bool{}
	list := []AnimeGardenGroup{}
	for _, group := range groups {
		if seen[group.ID] {
			continue
		}
		seen[group.ID] = true
		items := groupItems[group.ID]
		titles := make([]string, 0, len(items))
		for _, item := range items {
			titles = append(titles, item.Title)
		}
		group.Items = items
		group.GroupRegex = toGroupRegex(titles)
		list = append(list, group)
	}
	return list, nil
}

func animeGardenGet(path string, out any) error {
	resp, err := httpc.Get(animeGardenHost + path).Do()
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
