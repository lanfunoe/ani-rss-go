// Package service — Mikan 番剧与字幕组
package service

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/items"
	"ani-rss-go/internal/store"
)

// MikanHost Mikan Host
func MikanHost() string {
	return cmp.Or(strings.TrimRight(config.Get().MikanHost, "/"), "https://mikanani.me")
}

// MikanGroupDetail 字幕组信息（含 groupRegex）
type MikanGroupDetail struct {
	Label      string             `json:"label"`
	SubgroupId string             `json:"subgroupId"`
	Rss        string             `json:"rss"`
	BgmUrl     string             `json:"bgmUrl"`
	UpdateDay  string             `json:"updateDay"`
	Items      []entity.MikanItem `json:"items"`
	GroupRegex entity.GroupRegex  `json:"groupRegex"`
}

// MikanGetInfo 解析 Mikan Bangumi 页面
func MikanGetInfo(bangumiId string) *entity.MikanInfo {
	host := MikanHost()
	pageURL := host + "/Home/Bangumi/" + bangumiId
	resp, err := httpc.Get(pageURL).Do()
	if err != nil {
		slog.Error("获取 Mikan 页面失败", "error", err)
		return nil
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		slog.Error("获取 Mikan 页面失败", "error", err)
		return nil
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		slog.Error("解析 Mikan 页面失败", "error", err)
		return nil
	}

	info := &entity.MikanInfo{URL: pageURL}
	if cover := doc.Find(".content > img").First(); cover.Length() > 0 {
		src, _ := cover.Attr("src")
		info.Cover = host + src
	}
	if title := doc.Find(".bangumi-title").First(); title.Length() > 0 {
		info.Title = strings.TrimSpace(title.Text())
	}

	groups, bgmUrl := parseMikanGroups(doc, host)
	info.BgmUrl = bgmUrl
	for _, g := range groups {
		info.Groups = append(info.Groups, entity.MikanGroup{
			Label:      g.Label,
			SubgroupId: g.SubgroupId,
			Rss:        g.Rss,
			UpdateDay:  g.UpdateDay,
			Items:      g.Items,
		})
	}
	return info
}

// MikanGroupList 解析指定 Mikan 页面的字幕组列表
func MikanGroupList(pageURL string) []MikanGroupDetail {
	resp, err := httpc.Get(pageURL).Do()
	if err != nil {
		slog.Error("获取 Mikan 页面失败", "error", err)
		return nil
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		slog.Error("解析 Mikan 页面失败", "error", err)
		return nil
	}
	groups, _ := parseMikanGroups(doc, MikanHost())
	for i := range groups {
		titles := make([]string, 0, len(groups[i].Items))
		for _, item := range groups[i].Items {
			titles = append(titles, item.Title)
		}
		groups[i].GroupRegex = toGroupRegex(titles)
	}
	return groups
}

// parseMikanGroups 解析页面中的字幕组行（.leftbar-item），返回 bgm 链接与字幕组列表
func parseMikanGroups(doc *goquery.Document, host string) ([]MikanGroupDetail, string) {
	bgmUrl := ""
	doc.Find(".bangumi-info").Each(func(i int, s *goquery.Selection) {
		if strings.Contains(s.Text(), "Bangumi番组计划链接") {
			if a := s.Find("a").First(); a.Length() > 0 {
				href, _ := a.Attr("href")
				bgmUrl = href
			}
		}
	})

	var groups []MikanGroupDetail
	doc.Find(".leftbar-item").Each(func(i int, sg *goquery.Selection) {
		a := sg.Find("a.subgroup-name").First()
		if a.Length() == 0 {
			return
		}
		// 无 data-anchor 的行无法定位 RSS 与资源表格，跳过
		anchor, _ := a.Attr("data-anchor")
		anchor = strings.TrimSpace(anchor)
		if anchor == "" {
			return
		}
		label := strings.TrimSpace(a.Text())
		subgroupId := strings.TrimPrefix(anchor, "#")
		rssHref := ""
		var items []entity.MikanItem
		if target := doc.Find(anchor).First(); target.Length() > 0 {
			if rss := target.Find(".mikan-rss").First(); rss.Length() > 0 {
				rssHref, _ = rss.Attr("href")
			}
			// 解析字幕组表格中的条目
			table := target.Next()
			table.Find("tbody tr").Each(func(j int, tr *goquery.Selection) {
				as := tr.Find("a")
				if as.Length() < 3 {
					return
				}
				title := ownTextOf(as.Eq(0))
				magnet, _ := as.Eq(1).Attr("data-clipboard-text")
				torrent, _ := as.Eq(2).Attr("href")
				tds := tr.Find("td")
				formatSize := ""
				dateStr := ""
				if tds.Length() >= 4 {
					formatSize = strings.TrimSpace(tds.Eq(2).Text())
					dateStr = strings.TrimSpace(tds.Eq(3).Text())
				}
				items = append(items, entity.MikanItem{
					Title:      title,
					Magnet:     magnet,
					FormatSize: formatSize,
					CreatedAt:  dateStr,
					Torrent:    host + torrent,
				})
			})
		}

		groups = append(groups, MikanGroupDetail{
			Label:      label,
			SubgroupId: subgroupId,
			Rss:        MikanHost() + rssHref,
			BgmUrl:     bgmUrl,
			UpdateDay:  strings.TrimSpace(sg.Find(".date").Text()),
			Items:      items,
		})
	})
	return groups, bgmUrl
}

// ownTextOf 获取选中元素的直属文本（不含子元素）
func ownTextOf(s *goquery.Selection) string {
	clone := s.Clone()
	clone.Children().Remove()
	return strings.TrimSpace(clone.Text())
}

// MikanSeason 季度筛选
type MikanSeason struct {
	Year        int    `json:"year"`
	Season      string `json:"season"`
	SeasonLabel string `json:"seasonLabel"`
	Select      *bool  `json:"select"`
}

type mikanSearchItem struct {
	BgmId  string  `json:"bgmId"`
	Cover  string  `json:"cover"`
	Title  string  `json:"title"`
	URL    string  `json:"url"`
	Exists bool    `json:"exists"`
	Score  float64 `json:"score"`
}

type mikanSearchWeek struct {
	WeekLabel string            `json:"weekLabel"`
	Items     []mikanSearchItem `json:"items"`
}

type mikanSearchResult struct {
	Seasons   []MikanSeason     `json:"seasons"`
	Weeks     []mikanSearchWeek `json:"weeks"`
	TotalItem int               `json:"totalItem"`
}

var mikanIdTailReg = regexp.MustCompile(`\d+(/)?$`)

var mikanIdSearchReg = regexp.MustCompile(`^id: (\d+)$`)

// MikanSearch 搜索 mikan 番剧列表，并用捐赠缓存补充评分与 BGM ID
func MikanSearch(text string, season *MikanSeason) any {
	var mikanBgmMap map[string]mikanBgm
	var wg sync.WaitGroup
	wg.Go(func() { mikanBgmMap = donateMikanBgm() })
	result := mikanSearch(text, season)
	wg.Wait()

	bgmIdSet := subscribedBgmIds()
	for _, week := range result.Weeks {
		for i := range week.Items {
			item := &week.Items[i]
			mb, exists := mikanBgmMap[mikanIdTailReg.FindString(item.URL)]
			if !exists {
				continue
			}
			item.Score = mb.Score
			item.BgmId = mb.BgmId
			if bgmIdSet[mb.BgmId] {
				item.Exists = true
			}
		}
		slices.SortStableFunc(week.Items, func(a, b mikanSearchItem) int {
			return cmp.Compare(b.Score, a.Score)
		})
	}
	return result
}

// mikanSearch 解析 mikan 搜索或季度浏览结果
func mikanSearch(text string, season *MikanSeason) mikanSearchResult {
	bangumiIdSet := map[string]bool{}
	for _, ani := range store.ANIList() {
		if id := items.RssParam(ani.URL, "bangumiId"); id != "" {
			bangumiIdSet[id] = true
		}
	}

	seasons := []MikanSeason{}
	weeks := []mikanSearchWeek{}

	// 指定 mikan id
	if m := mikanIdSearchReg.FindStringSubmatch(text); len(m) > 1 {
		info := MikanGetInfo(m[1])
		items := []mikanSearchItem{}
		if info != nil {
			items = append(items, mikanSearchItem{
				Cover: info.Cover,
				Title: info.Title,
				URL:   info.URL,
			})
		}
		weeks = append(weeks, mikanSearchWeek{WeekLabel: "Search", Items: items})
		return mikanSearchResult{Seasons: seasons, Weeks: weeks, TotalItem: len(items)}
	}

	// 无关键字且未指定季度时抓取首页（当前季度）
	host := MikanHost()
	pageURL := host
	if strings.TrimSpace(text) != "" {
		pageURL = host + "/Home/Search?searchstr=" + url.QueryEscape(text)
	} else if season != nil && season.Year != 0 && season.Season != "" {
		pageURL = fmt.Sprintf("%s/Home/BangumiCoverFlowByDayOfWeek?year=%d&seasonStr=%s", host, season.Year, url.QueryEscape(season.Season))
	}
	seasons, weeks = mikanScrapePage(pageURL, host, bangumiIdSet)

	totalItem := 0
	for _, week := range weeks {
		totalItem += len(week.Items)
	}
	return mikanSearchResult{Seasons: seasons, Weeks: weeks, TotalItem: totalItem}
}

// mikanScrapePage 解析 mikan 搜索/季度浏览页面
func mikanScrapePage(pageURL, host string, bangumiIdSet map[string]bool) ([]MikanSeason, []mikanSearchWeek) {
	seasons := []MikanSeason{}
	weeks := []mikanSearchWeek{}
	resp, err := httpc.Get(pageURL).Do()
	if err != nil {
		slog.Error("获取 Mikan 页面失败", "error", err)
		return seasons, weeks
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		slog.Error("解析 Mikan 页面失败", "error", err)
		return seasons, weeks
	}

	// 季度下拉框
	dateSelect := doc.Find(".date-select").First()
	if dateSelect.Length() > 0 {
		dateText := strings.TrimSpace(dateSelect.Find(".date-text").First().Text())
		// 顶层菜单的每个子项为一个年份，其下的 li 为该年的各季度
		dateSelect.Find(".dropdown-menu").First().Children().Each(func(_ int, child *goquery.Selection) {
			child.Find("li").Each(func(_ int, li *goquery.Selection) {
				a := li.Find("a").First()
				if a.Length() == 0 {
					return
				}
				dataYear, _ := a.Attr("data-year")
				dataSeason, _ := a.Attr("data-season")
				year, err := strconv.Atoi(dataYear)
				if err != nil {
					return
				}
				label := dataYear + " " + dataSeason
				selected := strings.HasPrefix(dateText, label)
				seasons = append(seasons, MikanSeason{
					Year:        year,
					Season:      dataSeason,
					SeasonLabel: label,
					Select:      &selected,
				})
			})
		})
	}

	// 解析条目列表
	parseItems := func(el *goquery.Selection) []mikanSearchItem {
		items := []mikanSearchItem{}
		if el.Length() == 0 {
			return items
		}
		el.Find("li").Each(func(_ int, li *goquery.Selection) {
			cover := ""
			if span := li.Find("span").First(); span.Length() > 0 {
				if src, ok := span.Attr("data-src"); ok {
					cover = host + src
				}
			}
			a := li.Find("a").First()
			if a.Length() == 0 {
				return
			}
			href, _ := a.Attr("href")
			href = host + href
			id := mikanIdTailReg.FindString(href)
			items = append(items, mikanSearchItem{
				Cover:  cover,
				Title:  strings.TrimSpace(a.Text()),
				URL:    href,
				Exists: bangumiIdSet[id],
			})
		})
		return items
	}

	skBangumis := doc.Find(".sk-bangumi")
	if skBangumis.Length() == 0 {
		weeks = append(weeks, mikanSearchWeek{WeekLabel: "Search", Items: parseItems(doc.Find(".an-ul").First())})
	} else {
		skBangumis.Each(func(_ int, sk *goquery.Selection) {
			items := parseItems(sk)
			if len(items) == 0 {
				// 番剧为空
				return
			}
			// 星期
			label := ""
			if sk.Children().Length() > 0 {
				label = strings.TrimSpace(sk.Children().Eq(0).Text())
			}
			weeks = append(weeks, mikanSearchWeek{WeekLabel: label, Items: items})
		})
	}
	return seasons, weeks
}

// MikanFillAni 根据字幕组 ID 填充订阅信息
func MikanFillAni(ani *entity.Ani, subgroupId string) error {
	bangumiId := items.RssParam(ani.URL, "bangumiId")
	if bangumiId == "" {
		return nil
	}
	info := MikanGetInfo(bangumiId)
	if info == nil {
		return errors.New("获取 Mikan 信息失败")
	}
	ani.MikanTitle = info.Title
	ani.BgmUrl = info.BgmUrl
	for _, g := range info.Groups {
		if g.SubgroupId == subgroupId {
			ani.Subgroup = g.Label
		}
	}
	return nil
}
