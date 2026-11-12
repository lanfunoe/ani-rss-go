// Package items RSS 抓取与资源解析
package items

import (
	"cmp"
	"encoding/xml"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/notification"
	"ani-rss-go/internal/rename"
	"ani-rss-go/internal/utils"
)

// rssItem 原始 RSS 条目
type rssItem struct {
	Title     string `xml:"title"`
	Enclosure struct {
		URL    string `xml:"url,attr"`
		Length string `xml:"length,attr"`
	} `xml:"enclosure"`
	GUID        string        `xml:"guid"`
	Link        string        `xml:"link"`
	PubDate     string        `xml:"pubDate"`
	InfoHash    string        `xml:"infoHash"`
	Size        string        `xml:"size"`
	TorrentInfo *torrentExtra `xml:"torrent"`
}

type torrentExtra struct {
	InfoHash      string `xml:"infohash"`
	PubDate       string `xml:"pubDate"`
	ContentLength string `xml:"contentLength"`
	MagnetURI     string `xml:"magneturi"`
}

type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

// GetItems 获取订阅的全部资源（主 RSS + 备用 RSS，去重排序）
func GetItems(ani *entity.Ani) ([]*entity.Item, error) {
	subgroup := cmp.Or(strings.TrimSpace(ani.Subgroup), entity.SubgroupUnknown)
	list, err := GetItemsByRssPublic(ani, ani.URL, subgroup)
	if err != nil {
		return nil, err
	}
	for _, it := range list {
		it.Master = new(true)
	}

	cfg := config.Get()
	if !cfg.GetStandbyRss() {
		sortByEpisode(list)
		return list, nil
	}

	for _, rss := range ani.StandbyRssList {
		time.Sleep(time.Second)
		label := cmp.Or(strings.TrimSpace(rss.Label), entity.SubgroupUnknown)
		clone := *ani
		clone.Offset = rss.Offset
		sub, err := GetItemsByRssPublic(&clone, rss.URL, label)
		if err != nil {
			return nil, err
		}
		for _, it := range sub {
			it.Master = new(false)
		}
		list = append(list, sub...)
	}

	// 去重：多字幕组共存模式按重命名去重，否则按集数去重（保留先出现的）
	coexist := cfg.GetCoexist()
	seen := map[string]bool{}
	var deduped []*entity.Item
	for _, it := range list {
		key := it.ReName
		if !coexist {
			key = strconv.FormatFloat(it.Episode, 'f', -1, 64)
		}
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, it)
		}
	}
	sortByEpisode(deduped)
	return deduped, nil
}

func sortByEpisode(items []*entity.Item) {
	slices.SortStableFunc(items, func(a, b *entity.Item) int { return cmp.Compare(a.Episode, b.Episode) })
}

// GetItemsByRssPublic 抓取单个 RSS 并解析过滤
func GetItemsByRssPublic(ani *entity.Ani, rssURL, subgroupName string) ([]*entity.Item, error) {
	xmlData, err := GetRss(rssURL)
	if err != nil {
		return nil, err
	}
	var feed rssFeed
	if err := xml.Unmarshal([]byte(xmlData), &feed); err != nil {
		return nil, fmt.Errorf("解析 RSS 失败, %s: %w", rssURL, err)
	}

	cfg := config.Get()
	globalExclude := ani.GetGlobalExclude()

	// {{字幕组}}:规则 前缀处理
	mapFn := func(s string) string {
		m := entity.SubgroupRe.FindStringSubmatch(s)
		if len(m) == 0 {
			return s
		}
		if m[1] == subgroupName {
			return m[2]
		}
		return ""
	}
	// 预编译正则，编译失败的条目视作不匹配
	compile := func(list []string) []*regexp.Regexp {
		var out []*regexp.Regexp
		for _, s := range list {
			s = mapFn(s)
			if s == "" {
				continue
			}
			re, err := regexp.Compile(s)
			if err != nil {
				slog.Warn("正则编译失败", "regex", s, "error", err)
				out = append(out, nil)
				continue
			}
			out = append(out, re)
		}
		return out
	}
	excludeRes := compile(ani.Exclude)
	matchRes := compile(ani.Match)
	globalExcludeRes := compile(cfg.Exclude)

	// 从最后一条开始解析，较早发布的条目排在前面
	var items []*entity.Item
	for _, ri := range slices.Backward(feed.Channel.Items) {
		itemTitle := ri.Title
		torrent := ri.Enclosure.URL
		length := ri.Enclosure.Length
		infoHash := ""
		formatSize := "0MiB"
		var pubDate *time.Time

		if m := entity.MagnetReg.FindStringSubmatch(torrent); m != nil {
			infoHash = m[1]
		}
		if m := entity.Ed2kReg.FindStringSubmatch(torrent); m != nil {
			infoHash = m[3]
		}
		// guid 为纯字母数字时视作 infoHash
		if ri.GUID != "" && isAlnum(ri.GUID) {
			infoHash = ri.GUID
		}
		if ri.InfoHash != "" {
			infoHash = ri.InfoHash
		}
		if ri.Size != "" {
			formatSize = ri.Size
		}
		if ri.PubDate != "" {
			if t, err := httpParseTime(ri.PubDate); err == nil {
				pubDate = &t
			}
		}
		if ri.TorrentInfo != nil {
			if ri.TorrentInfo.InfoHash != "" {
				infoHash = ri.TorrentInfo.InfoHash
			}
			if pubDate == nil && ri.TorrentInfo.PubDate != "" {
				if t, err := httpParseTime(ri.TorrentInfo.PubDate); err == nil {
					pubDate = &t
				}
			}
			if length == "" {
				length = ri.TorrentInfo.ContentLength
			}
			if torrent == "" {
				torrent = ri.TorrentInfo.MagnetURI
			}
		}
		// link 指向 .torrent 时作为种子地址
		if strings.HasSuffix(ri.Link, ".torrent") {
			torrent = ri.Link
		}

		if torrent == "" {
			continue
		}
		if infoHash == "" {
			// 取文件名主体
			infoHash = torrent
			if _, name, ok := strings.CutLast(torrent, "/"); ok {
				infoHash = name
			}
			infoHash, _, _ = strings.Cut(infoHash, "?")
			infoHash = strings.TrimSuffix(infoHash, ".torrent")
		}
		infoHash = strings.ToLower(urlDecode(infoHash))

		if formatSize == "0MiB" {
			size := int64(1)
			if n, err := strconv.ParseInt(length, 10, 64); err == nil && n >= 0 {
				size = n
			}
			formatSize = utils.FormatSize(size, true)
		}

		item := &entity.Item{
			Title:      itemTitle,
			ReName:     itemTitle,
			Torrent:    torrent,
			InfoHash:   infoHash,
			FormatSize: formatSize,
			PubDate:    pubDate,
			Subgroup:   subgroupName,
			Episode:    1.0,
		}

		// 排除、匹配（需全部命中）、全局排除
		matched := func(re *regexp.Regexp) bool { return re != nil && re.MatchString(item.Title) }
		unmatched := func(re *regexp.Regexp) bool { return !matched(re) }
		if slices.ContainsFunc(excludeRes, matched) ||
			slices.ContainsFunc(matchRes, unmatched) ||
			globalExclude && slices.ContainsFunc(globalExcludeRes, matched) {
			continue
		}
		items = append(items, item)
	}

	// 计算重命名与集数
	var filtered []*entity.Item
	for _, item := range items {
		if rename.Rename(ani, item) {
			filtered = append(filtered, item)
		}
	}

	// 按集数去重，保留首次出现的位置，条目取 RSS 中靠前（较新）的一条
	index := map[float64]int{}
	var out []*entity.Item
	for _, item := range filtered {
		if i, exists := index[item.Episode]; exists {
			out[i] = item
			continue
		}
		index[item.Episode] = len(out)
		out = append(out, item)
	}
	return out, nil
}

// RSS 常见时间格式
var timeLayouts = []string{
	time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05", "2006-01-02",
}

// httpParseTime 解析 RFC1123、ISO 8601 等 RSS 常见时间格式，不含时区时按本地时间解析
func httpParseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range timeLayouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间: %s", s)
}

// isAlnum 是否为非空的小写字母与数字组合
func isAlnum(s string) bool {
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
}

// GetRss 获取 RSS 内容并校验为 XML
func GetRss(rssURL string) (string, error) {
	timeout := time.Duration(utils.Deref(config.Get().RssTimeout)) * time.Second
	resp, err := httpc.Get(rssURL).SetTimeout(timeout).Do()
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return "", err
	}
	if err := resp.AssertXml(); err != nil {
		return "", err
	}
	body, err := resp.BodyString()
	if err != nil {
		return "", fmt.Errorf("读取 RSS 失败, %s: %w", rssURL, err)
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return "", fmt.Errorf("RSS 内容为空, %s", rssURL)
	}
	if !strings.HasPrefix(body, "<") {
		return "", fmt.Errorf("RSS 内容非 XML, %s", rssURL)
	}
	return body, nil
}

// OmitList 计算缺失的集数
func OmitList(ani *entity.Ani, items []*entity.Item) []int {
	var list []int
	cfg := config.Get()
	if !cfg.GetOmit() || len(items) == 0 || !ani.GetOmit() || ani.GetOva() {
		return list
	}
	set := make(map[int]bool)
	first, last := math.MaxInt, 0
	for _, it := range items {
		ep := int(it.Episode)
		set[ep] = true
		first, last = min(first, ep), max(last, ep)
	}
	if first == last {
		return list
	}
	for ep := first; ep <= last; ep++ {
		if set[ep] {
			continue
		}
		if len(list) > 50 {
			return list
		}
		list = append(list, ep)
	}
	return list
}

// Omit 缺集检测并通知
func Omit(ani *entity.Ani, items []*entity.Item) {
	list := OmitList(ani, items)
	if len(list) == 0 || len(list) > 10 {
		return
	}
	var sList []string
	for _, ep := range list {
		s := fmt.Sprintf("缺少集数 %s S%02dE%02d", ani.Title, ani.Season, ep)
		key := fmt.Sprintf("omit:%s:ep-%d", ani.ID, ep)
		if utils.Cache.Has(key) {
			continue
		}
		slog.Info(s)
		utils.Cache.Set(key, s, 24*time.Hour)
		sList = append(sList, s)
	}
	if len(sList) == 0 {
		return
	}
	notification.Send(config.Get(), ani, strings.Join(sList, "\n"), entity.StatusOmit)
}

// CurrentEpisodeNumber 当前集数
func CurrentEpisodeNumber(ani *entity.Ani, items []*entity.Item) int {
	cfg := config.Get()
	if cfg.GetStandbyRss() && cfg.GetCoexist() {
		items = masterItems(items)
	}
	var nums []int
	for _, it := range items {
		if !Is5(it.Episode) {
			nums = append(nums, int(it.Episode))
		}
	}
	if len(nums) == 0 {
		return 0
	}
	if ani.GetDownloadNew() {
		return max(0, slices.Max(nums))
	}
	return len(nums)
}

// Procrastinating 摸鱼检测
func Procrastinating(ani *entity.Ani, items []*entity.Item) {
	cfg := config.Get()
	if !cfg.GetProcrastinating() || !ani.GetProcrastinating() {
		return
	}
	if cfg.GetProcrastinatingMasterOnly() {
		items = masterItems(items)
	}
	// 已完结 不再检测摸鱼
	if ani.TotalEpisodeNumber > 0 && CurrentEpisodeNumber(ani, items) >= ani.TotalEpisodeNumber {
		return
	}
	var latest int64
	for _, it := range items {
		if it.PubDate != nil && it.PubDate.UnixMilli() > latest {
			latest = it.PubDate.UnixMilli()
		}
	}
	if latest == 0 {
		return
	}
	now := time.Now().UnixMilli()
	if now <= latest {
		return
	}
	day := int((now - latest) / (24 * 3600 * 1000))
	if utils.Deref(cfg.ProcrastinatingDay) > day {
		return
	}
	text := fmt.Sprintf("检测到%s, 已摸鱼%d天", ani.Title, day)
	key := "procrastinating:" + ani.ID
	if utils.Cache.Has(key) {
		return
	}
	utils.Cache.Set(key, text, 24*time.Hour)
	notification.Send(cfg, ani, text, entity.StatusProcrastinating)
}

// masterItems 主 RSS 的资源
func masterItems(items []*entity.Item) []*entity.Item {
	var master []*entity.Item
	for _, it := range items {
		if it.GetMaster() {
			master = append(master, it)
		}
	}
	return master
}

// GetSubgroup 从标题列表推断字幕组
func GetSubgroup(items []*entity.Item) string {
	for _, item := range items {
		title := item.Title
		if !entity.SubgroupTag.MatchString(title) {
			if idx := strings.LastIndexAny(title, "/\\"); idx >= 0 {
				title = title[idx+1:]
			}
		}
		if m := entity.SubgroupTag.FindStringSubmatch(title); len(m) > 1 {
			return m[1]
		}
	}
	return entity.SubgroupUnknown
}

// Is5 是否为 x.5 集
func Is5(episode float64) bool {
	return episode != float64(int(episode))
}

func urlDecode(s string) string {
	if d, err := url.QueryUnescape(s); err == nil {
		return d
	}
	return s
}

// RssParam 提取 RSS 地址中的查询参数，参数名忽略大小写，地址无法解析时也按原文查找
func RssParam(rawURL, key string) string {
	_, after, ok := strings.Cut(rawURL, "?")
	if !ok {
		return ""
	}
	for kv := range strings.SplitSeq(after, "&") {
		k, v, ok := strings.Cut(kv, "=")
		if ok && strings.EqualFold(k, key) {
			if unescaped, err := url.QueryUnescape(v); err == nil {
				return unescaped
			}
			return v
		}
	}
	return ""
}
