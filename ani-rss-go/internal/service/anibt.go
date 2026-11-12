// Package service — AniBT 番剧浏览
package service

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/utils"
)

const aniBTHost = "https://anibt.net"

// AniBTQuery AniBT 查询参数
type AniBTQuery struct {
	Season string `json:"season"`
	BgmUrl string `json:"bgmUrl"`
	Title  string `json:"title"`
}

// AniBT AniBT 季度番剧列表
type AniBT struct {
	CurrentSeason    string           `json:"currentSeason"`
	RequestedSeason  string           `json:"requestedSeason"`
	AvailableSeasons []string         `json:"availableSeasons"`
	ByWeekday        []AniBTByWeekday `json:"byWeekday"`
}

type AniBTByWeekday struct {
	Animes       []AniBTAnime `json:"animes"`
	Weekday      int          `json:"weekday"`
	WeekdayLabel string       `json:"weekdayLabel"`
}

type AniBTAnime struct {
	AnimeId         string            `json:"animeId"`
	BgmId           entity.FlexString `json:"bgmId"`
	Cover           string            `json:"cover"`
	Rating          float64           `json:"rating"`
	Title           AniBTTitle        `json:"title"`
	Format          string            `json:"format"`
	Exists          bool              `json:"exists"`
	RssReleaseCount int               `json:"rssReleaseCount"`
}

type AniBTTitle struct {
	Chinese            string `json:"chinese"`
	ChineseTraditional string `json:"chineseTraditional"`
	English            string `json:"english"`
	Primary            string `json:"primary"`
	Romaji             string `json:"romaji"`
}

type AniBTGroup struct {
	BgmId         string            `json:"bgmId"`
	GroupId       string            `json:"groupId"`
	Slug          string            `json:"slug"`
	Name          string            `json:"name"`
	Status        string            `json:"status"`
	LastUpdatedAt int64             `json:"lastUpdatedAt"`
	Items         []AniBTItem       `json:"items"`
	Rss           string            `json:"rss"`
	GroupRegex    entity.GroupRegex `json:"groupRegex"`
}

type AniBTItem struct {
	EpisodeKey  string   `json:"episodeKey"`
	Language    []string `json:"language"`
	Magnet      string   `json:"magnet"`
	PublishedAt int64    `json:"publishedAt"`
	ReleaseId   string   `json:"releaseId"`
	Resolution  string   `json:"resolution"`
	Subtitle    string   `json:"subtitle"`
	Title       string   `json:"title"`
	Size        int64    `json:"size"`
	FormatSize  string   `json:"formatSize"`
}

// AniBTList 获取 AniBT 番剧列表，按评分排序并标记已订阅
func AniBTList(query *AniBTQuery) (*AniBT, error) {
	season, bgmId := query.Season, ""
	if query.BgmUrl != "" {
		bgmId = entity.SubjectIdFromUrl(query.BgmUrl)
	}
	if query.Title != "" {
		season, bgmId = "", ""
	}
	params := url.Values{}
	params.Set("season", season)
	params.Set("bgmId", bgmId)
	params.Set("query", query.Title)

	out := struct {
		Data AniBT `json:"data"`
	}{}
	if err := aniBTGet("/api/seasons/anime?"+params.Encode(), &out); err != nil {
		return nil, err
	}
	aniBT := out.Data

	bgmIdSet := subscribedBgmIds()
	weekdays := make([]AniBTByWeekday, 0, len(aniBT.ByWeekday))
	for _, weekday := range aniBT.ByWeekday {
		animes := make([]AniBTAnime, 0, len(weekday.Animes))
		for _, anime := range weekday.Animes {
			if query.Title == "" && anime.RssReleaseCount <= 0 {
				continue
			}
			anime.Exists = bgmIdSet[string(anime.BgmId)]
			animes = append(animes, anime)
		}
		if len(animes) == 0 {
			continue
		}
		slices.SortStableFunc(animes, func(a, b AniBTAnime) int {
			return cmp.Compare(b.Rating, a.Rating)
		})
		weekday.Animes = animes
		weekdays = append(weekdays, weekday)
	}
	slices.SortStableFunc(weekdays, func(a, b AniBTByWeekday) int {
		return cmp.Compare(weekOrder(a.WeekdayLabel), weekOrder(b.WeekdayLabel))
	})
	aniBT.ByWeekday = weekdays
	return &aniBT, nil
}

// AniBTGroups 获取番剧的字幕组列表
func AniBTGroups(bgmId string) ([]AniBTGroup, error) {
	out := struct {
		Data struct {
			Groups []AniBTGroup `json:"groups"`
		} `json:"data"`
	}{}
	if err := aniBTGet("/api/anime/groups?bgmId="+url.QueryEscape(bgmId), &out); err != nil {
		return nil, err
	}
	groups := out.Data.Groups
	for i := range groups {
		group := &groups[i]
		group.Rss = fmt.Sprintf("%s/rss/anime.xml?bgmId=%s&groupSlug=%s", aniBTHost, bgmId, group.Slug)
		titles := make([]string, 0, len(group.Items))
		for j := range group.Items {
			group.Items[j].FormatSize = utils.FormatSize(group.Items[j].Size, true)
			titles = append(titles, group.Items[j].Title)
		}
		group.BgmId = bgmId
		group.GroupRegex = toGroupRegex(titles)
	}
	if groups == nil {
		groups = []AniBTGroup{}
	}
	return groups, nil
}

func aniBTGet(path string, out any) error {
	resp, err := httpc.Get(aniBTHost + path).Do()
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
