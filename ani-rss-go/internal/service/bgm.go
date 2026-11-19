// Package service — Bangumi API
package service

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/download"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/items"
	"ani-rss-go/internal/rename"
	"ani-rss-go/internal/store"
	"ani-rss-go/internal/utils"
)

// GetBgmInfo 获取订阅对应的 BGM 条目信息，无法取得条目 ID 或请求失败时返回错误
func GetBgmInfo(ani *entity.Ani, useCache bool) (*entity.BgmInfo, error) {
	subjectId := GetSubjectId(ani)
	if subjectId == "" {
		slog.Warn("未获取到 bgmId", "title", ani.Title)
		return nil, errors.New("无法取得 subjectId")
	}
	info := bgmInfoBySubjectId(subjectId, useCache)
	if info == nil {
		return nil, errors.New("获取 BGM 信息失败")
	}
	return info, nil
}

// bgmInfoBySubjectId 获取 BGM 条目信息，useCache 为 true 时并行请求 cache.wushuo.top，
// BGM API 失败时使用其结果
func bgmInfoBySubjectId(subjectId string, useCache bool) *entity.BgmInfo {
	var info, cached *entity.BgmInfo
	var apiErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		info, apiErr = fetchBgmInfo(bgmAuth(httpc.Get(bgmApiHost() + "/v0/subjects/" + subjectId)))
	})
	if useCache {
		wg.Go(func() {
			cached, _ = fetchBgmInfo(httpc.Get("https://cache.wushuo.top/bgm/subjects/" + subjectId))
		})
	}
	wg.Wait()
	if info != nil {
		return info
	}
	slog.Error("获取 BGM 信息失败", "subjectId", subjectId, "error", apiErr)
	return cached
}

// fetchBgmInfo 请求 BGM 条目信息，清洗标题并推断季数
func fetchBgmInfo(req *httpc.Request) (*entity.BgmInfo, error) {
	resp, err := req.SetTimeout(10 * time.Second).Do()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return nil, err
	}
	info := &entity.BgmInfo{}
	if err := json.NewDecoder(resp.Body).Decode(info); err != nil {
		return nil, err
	}
	info.Season = getSeasonByBgmInfo(info)
	info.Name = rename.CleanName(info.Name)
	info.NameCn = rename.CleanName(info.NameCn)
	if info.Date == nil {
		info.Date = &entity.Date{Time: time.Now()}
	}
	return info, nil
}

// SearchBgm 搜索 BGM 条目，请求失败时返回空列表
func SearchBgm(name string) []*entity.BgmInfo {
	list := []*entity.BgmInfo{}
	if strings.TrimSpace(name) == "" {
		return list
	}
	name = strings.ReplaceAll(name, "1/2", "½")
	resp, err := bgmAuth(httpc.Get(bgmApiHost() + "/search/subject/" + url.PathEscape(name) + "?type=2&max_results=25&responseGroup=small")).Do()
	if err != nil {
		return list
	}
	defer resp.Body.Close()
	if resp.AssertStatus() != nil {
		return list
	}
	out := struct {
		Code *int              `json:"code"`
		List []*entity.BgmInfo `json:"list"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return list
	}
	if utils.Deref(out.Code) == 404 {
		return list
	}
	for _, info := range out.List {
		info.Season = getSeasonByBgmInfo(info)
		list = append(list, info)
	}
	return list
}

// GetSubjectId 获取订阅的 BGM 条目 ID，蜜柑订阅缺少 bgmUrl 时先从蜜柑番剧页补全
func GetSubjectId(ani *entity.Ani) string {
	if ani == nil {
		return ""
	}
	if ani.BgmUrl != "" || ani.Type != "mikan" || items.RssParam(ani.URL, "bangumiId") == "" {
		return entity.SubjectIdFromUrl(ani.BgmUrl)
	}
	filled := *ani
	if err := MikanFillAni(&filled, ""); err != nil || filled.BgmUrl == "" {
		return ""
	}
	if tx := store.BeginAni(ani.ID); tx != nil {
		if tx.BgmUrl == "" && tx.URL == ani.URL {
			tx.MikanTitle, tx.BgmUrl = filled.MikanTitle, filled.BgmUrl
		}
		tx.Commit()
	}
	return entity.SubjectIdFromUrl(filled.BgmUrl)
}

// GetEps 获取总集数，优先使用正片集列表数量排除 OVA
func GetEps(info *entity.BgmInfo) int {
	if info == nil || info.Eps < 1 {
		return 0
	}
	if n := len(bgmEpisodes(strconv.Itoa(info.ID))); n > 0 {
		return n
	}
	return info.Eps
}

type bgmEpisode struct {
	ID     int     `json:"id"`
	Ep     float64 `json:"ep"`
	Sort   float64 `json:"sort"`
	Name   string  `json:"name"`
	NameCn string  `json:"name_cn"`
}

// bgmEpisodes 获取正常集列表（type 0）
func bgmEpisodes(subjectId string) []bgmEpisode {
	time.Sleep(500 * time.Millisecond)
	resp, err := bgmAuth(httpc.Get(bgmApiHost() + "/v0/episodes?subject_id=" + subjectId + "&type=0&limit=1000&offset=0")).Do()
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.AssertStatus() != nil {
		return nil
	}
	out := struct {
		Data []bgmEpisode `json:"data"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil
	}
	return out.Data
}

type bgmEpisodeTitles struct {
	Title   string
	JpTitle string
}

// bgmEpisodeNumber 集数：BGM_OFFSET 时取跨季连续的 sort，否则取本季的 ep
func bgmEpisodeNumber(ep bgmEpisode) float64 {
	if config.Get().OffsetType == entity.OffsetBgm {
		return ep.Sort
	}
	return ep.Ep
}

// BgmEpisodeTitleMap 获取每集标题（中文与日文两套）
func BgmEpisodeTitleMap(ani *entity.Ani) map[int]bgmEpisodeTitles {
	m := map[int]bgmEpisodeTitles{}
	if ani == nil || ani.GetOva() {
		return m
	}
	subjectId := GetSubjectId(ani)
	if subjectId == "" {
		return m
	}
	key := "BGM_getEpisodeTitleMap:" + subjectId + ":" + config.Get().OffsetType
	if v, ok := utils.CacheGet[map[int]bgmEpisodeTitles](key); ok {
		return v
	}
	for _, ep := range bgmEpisodes(subjectId) {
		jpTitle := rename.CleanName(ep.Name)
		title := ep.NameCn
		if strings.TrimSpace(title) == "" {
			title = ep.Name
		}
		title = rename.CleanName(title)

		epNum := int(bgmEpisodeNumber(ep))
		defaultEpisodeTitle := "第" + strconv.Itoa(epNum) + "集"
		if strings.TrimSpace(title) == "" {
			title = defaultEpisodeTitle
		}
		if strings.TrimSpace(jpTitle) == "" {
			jpTitle = defaultEpisodeTitle
		}
		m[epNum] = bgmEpisodeTitles{Title: title, JpTitle: jpTitle}
	}
	utils.Cache.Set(key, m, 5*time.Minute)
	return m
}

// GetAniBySubjectId 将指定 id 的 BGM 番剧转换为订阅
func GetAniBySubjectId(id string) *entity.Ani {
	if id == "" {
		return nil
	}
	info := bgmInfoBySubjectId(id, true)
	if info == nil {
		return nil
	}
	ani := store.CreateAni()
	ToAni(info, ani)
	ani.CustomDownloadPath = new(true)
	return ani
}

// GetBgmTitle 获取 BGM 标题
func GetBgmTitle(ani *entity.Ani) (string, error) {
	bgmInfo, err := GetBgmInfo(ani, false)
	if err != nil {
		return "", err
	}
	return GetFinalNameWithTmdb(bgmInfo, ani.Tmdb), nil
}

// RefreshToken 刷新 BGM Token（AUTO 模式）
func RefreshToken() {
	cfg := config.Get()
	if cfg.BgmTokenType != entity.BgmTokenAuto {
		return
	}
	if cfg.BgmToken == "" {
		return
	}
	if bgmExpiresDays() >= 3 {
		return
	}
	if cfg.BgmAppID == "" || cfg.BgmAppSecret == "" || cfg.BgmRefreshToken == "" || cfg.BgmRedirectUri == "" {
		return
	}
	err := bgmOAuthToken(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     cfg.BgmAppID,
		"client_secret": cfg.BgmAppSecret,
		"refresh_token": cfg.BgmRefreshToken,
		"redirect_uri":  cfg.BgmRedirectUri,
	})
	if err != nil {
		slog.Error("刷新 BGM Token 失败", "error", err)
		return
	}
	slog.Info("BgmToken 已自动刷新")
}

// bgmExpiresDays 获取 BGM Token 剩余有效天数
func bgmExpiresDays() int {
	cfg := config.Get()
	if cfg.BgmToken == "" {
		return 0
	}
	resp, err := httpc.Post("https://bgm.tv/oauth/token_status").
		SetForm("access_token", cfg.BgmToken).
		Do()
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	out := struct {
		Expires int64 `json:"expires"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0
	}
	expires := out.Expires * 1000
	now := time.Now().UnixMilli()
	if expires <= now {
		return 0
	}
	return int(time.Duration(expires-now).Hours() / 24)
}

var cnNum = map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9, '十': 10, '百': 100, '千': 1000, '〇': 0, '零': 0}

// chineseToNumber 中文数字转阿拉伯数字
func chineseToNumber(s string) int {
	total, num := 0, 0
	for _, r := range s {
		v, ok := cnNum[r]
		if !ok {
			if n, err := strconv.Atoi(string(r)); err == nil {
				num = num*10 + n
			}
			continue
		}
		switch v {
		case 10, 100, 1000:
			if num == 0 {
				num = 1
			}
			total += num * v
			num = 0
		default:
			num = v
		}
	}
	return total + num
}

var seasonRes = []*regexp.Regexp{
	regexp.MustCompile(`第 ?([一二三四五六七八九十百千]+) ?[季期]`),
	regexp.MustCompile(`[Ss]eason ?(\d+)`),
	regexp.MustCompile(`(\d+)(st|nd|rd|th) ?[Ss]eason`),
	regexp.MustCompile(`[Ss](\d+)$`),
}

// GetSeasonByName 从名称中推断季数
func GetSeasonByName(name string) int {
	season := 1
	for _, re := range seasonRes {
		m := re.FindStringSubmatch(name)
		if len(m) < 2 {
			continue
		}
		if n, err := strconv.Atoi(m[1]); err == nil {
			season = n
		} else if n := chineseToNumber(m[1]); n > 0 {
			season = n
		}
	}
	return season
}

// getSeasonByBgmInfo 依次从标签、中文标题、原标题、别名推断季数
func getSeasonByBgmInfo(info *entity.BgmInfo) int {
	names := make([]string, 0, len(info.Tags)+2)
	for _, tag := range info.Tags {
		names = append(names, tag.Name)
	}
	names = append(names, info.NameCn, info.Name)
	for _, box := range info.Infobox {
		if box.Key != "别名" {
			continue
		}
		var values []struct {
			V string `json:"v"`
		}
		if json.Unmarshal(box.Value, &values) != nil {
			continue
		}
		for _, v := range values {
			names = append(names, v.V)
		}
	}
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if season := GetSeasonByName(name); season > 1 {
			return season
		}
	}
	return 1
}

// GetFinalName 计算 BGM 标题
func GetFinalName(info *entity.BgmInfo) string {
	title := info.NameCn
	if utils.Deref(config.Get().BgmJpName) || strings.TrimSpace(title) == "" {
		title = info.Name
	}
	return cmp.Or(strings.TrimSpace(title), "无标题")
}

// GetFinalNameWithTmdb 计算 BGM 标题并追加 tmdbid 与年份
func GetFinalNameWithTmdb(info *entity.BgmInfo, tmdb *entity.Tmdb) string {
	cfg := config.Get()
	title := GetFinalName(info)
	if utils.Deref(cfg.TitleYear) && info.Date != nil {
		title += fmt.Sprintf(" (%d)", info.Date.Year())
	}
	return TmdbFinalNameTitled(title, tmdb)
}

// ToAni 将 BGM 信息填充到订阅
func ToAni(info *entity.BgmInfo, ani *entity.Ani) {
	if info == nil || ani == nil {
		return
	}
	cfg := config.Get()
	image := info.Image(cfg.BgmImageSize)

	platform := strings.ToUpper(info.Platform)
	ova := platform == "OVA" || platform == "剧场版"

	title := GetFinalName(info)

	ani.BgmUrl = "https://bgm.tv/subject/" + strconv.Itoa(info.ID)
	// 标题
	ani.Title = title
	ani.JpTitle = info.Name
	// 季
	ani.Season = info.Season
	// 总集数
	ani.TotalEpisodeNumber = GetEps(info)
	// 剧场版
	ani.Ova = &ova
	// 评分
	ani.Score = 0
	if info.Rating != nil {
		ani.Score = info.Rating.Score
	}
	// 发布日期
	if info.Date != nil {
		ani.ReleaseDate = *info.Date
	}
	// 图片http地址
	ani.Image = image
	// 本地图片地址
	ani.Cover = store.SaveCover(image)

	// 获取tmdb标题
	themoviedbName := TmdbFinalName(ani)

	// 是否使用tmdb标题
	if themoviedbName != "" && utils.Deref(cfg.Tmdb) {
		ani.Title = themoviedbName
	} else {
		ani.Title = GetFinalNameWithTmdb(info, ani.Tmdb)
	}

	// 下载位置
	downloadPath := download.GetDownloadPath(ani)

	completedPathTemplate := cfg.CompletedPathTemplate

	if ova {
		// 剧场版默认不开启摸鱼检测
		ani.Procrastinating = new(false)
	}

	// tmdb 标题
	ani.ThemoviedbName = themoviedbName
	ani.CustomDownloadPathTemplate = downloadPath
	ani.CustomCompletedPathTemplate = completedPathTemplate
}

// bgmSleep 请求间隔，避免流控
func bgmSleep() {
	time.Sleep(time.Duration(500+rand.IntN(500)) * time.Millisecond)
}

// bgmApiHost 获取 BGM API 地址
func bgmApiHost() string {
	return cmp.Or(config.Get().BgmApi, "https://api.bgm.tv")
}

// bgmAuth 设置 BGM Token 并休眠以避免流控
func bgmAuth(req *httpc.Request) *httpc.Request {
	if token := config.Get().BgmToken; token != "" {
		req.Set("Authorization", "Bearer "+token)
	}
	bgmSleep()
	return req
}

// BgmMe 获取当前 BGM 账号信息
func BgmMe() (*entity.BgmMe, error) {
	token := config.Get().BgmToken
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("BgmToken 未填写")
	}
	resp, err := bgmAuth(httpc.Get(bgmApiHost() + "/v0/me")).Do()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return nil, err
	}
	raw := struct {
		Avatar     *entity.BgmAvatar `json:"avatar"`
		ID         int               `json:"id"`
		Sign       string            `json:"sign"`
		URL        string            `json:"url"`
		Username   string            `json:"username"`
		Nickname   string            `json:"nickname"`
		UserGroup  int               `json:"user_group"`
		RegTime    string            `json:"reg_time"`
		Email      string            `json:"email"`
		TimeOffset int               `json:"time_offset"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	return &entity.BgmMe{
		Avatar:     raw.Avatar,
		ID:         raw.ID,
		Sign:       raw.Sign,
		URL:        raw.URL,
		Username:   raw.Username,
		Nickname:   raw.Nickname,
		UserGroup:  raw.UserGroup,
		RegTime:    raw.RegTime,
		Email:      raw.Email,
		TimeOffset: raw.TimeOffset,
	}, nil
}

// BgmMeWithExpires 获取当前 BGM 账号信息与 Token 剩余天数
func BgmMeWithExpires() (*entity.BgmMe, error) {
	days := bgmExpiresDays()
	me, err := BgmMe()
	if err != nil {
		return nil, err
	}
	me.ExpiresDays = days
	return me, nil
}

// bgmUsername 获取用户名，为空时使用用户 ID
func bgmUsername() (string, error) {
	me, err := BgmMe()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(me.Username) != "" {
		return me.Username, nil
	}
	return strconv.Itoa(me.ID), nil
}

// BgmRate 获取或设置评分，rate 为 nil 时读取评分
func BgmRate(subjectId string, rate *int) (int, error) {
	if subjectId == "" {
		return 0, errors.New("无法取得 subjectId")
	}
	if rate == nil {
		username, err := bgmUsername()
		if err != nil {
			return 0, err
		}
		resp, err := bgmAuth(httpc.Get(bgmApiHost() + "/v0/users/" + username + "/collections/" + subjectId)).Do()
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == 404 {
			return 0, nil
		}
		if err := resp.AssertStatus(); err != nil {
			return 0, err
		}
		out := struct {
			Rate int `json:"rate"`
		}{}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return 0, err
		}
		return out.Rate, nil
	}
	body, _ := json.Marshal(map[string]int{"type": 3, "rate": *rate})
	resp, err := bgmAuth(httpc.Post(bgmApiHost()+"/v0/users/-/collections/"+subjectId)).
		Set("Content-Type", "application/json").
		SetBody(body).
		Do()
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return 0, err
	}
	return *rate, nil
}

// BgmCollections 收藏番剧为在看，已收藏时跳过
func BgmCollections(subjectId string) error {
	if subjectId == "" {
		return errors.New("subjectId 不能为空")
	}
	username, err := bgmUsername()
	if err != nil {
		return err
	}
	resp, err := bgmAuth(httpc.Get(bgmApiHost() + "/v0/users/" + username + "/collections/" + subjectId)).Do()
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.AssertStatus() == nil {
		slog.Info("已收藏番剧", "subjectId", subjectId)
		return nil
	}

	body, _ := json.Marshal(map[string]int{"type": 3})
	resp, err = bgmAuth(httpc.Post(bgmApiHost()+"/v0/users/-/collections/"+subjectId)).
		Set("Content-Type", "application/json").
		SetBody(body).
		Do()
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// BgmEpisodeId 根据集数获取 BGM 集 ID
func BgmEpisodeId(subjectId string, episode float64) string {
	for _, it := range bgmEpisodes(subjectId) {
		if bgmEpisodeNumber(it) == episode {
			return strconv.Itoa(it.ID)
		}
	}
	return ""
}

// bgmMinEpisode Bangumi 正篇最小集数，正篇列表为空时为 1
func bgmMinEpisode(ani *entity.Ani) int {
	episodes := bgmEpisodes(GetSubjectId(ani))
	if len(episodes) == 0 {
		return 1
	}
	return int(slices.MinFunc(episodes, func(a, b bgmEpisode) int { return cmp.Compare(a.Sort, b.Sort) }).Sort)
}

// BgmCollectionsEpisode 标记单集状态，type: 0 未看过, 1 想看, 2 看过
func BgmCollectionsEpisode(episodeId string, collectionType int) error {
	time.Sleep(500 * time.Millisecond)
	// 点格子前先判断状态，防止刷屏 #142
	resp, err := bgmAuth(httpc.Get(bgmApiHost() + "/v0/users/-/collections/-/episodes/" + episodeId)).Do()
	if err != nil {
		return err
	}
	now := struct {
		Type int `json:"type"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&now)
	resp.Body.Close()
	if err != nil {
		return err
	}
	if now.Type == collectionType {
		return nil
	}
	// 间隔 500 毫秒, 防止流控
	time.Sleep(500 * time.Millisecond)
	body, _ := json.Marshal(map[string]int{"type": collectionType})
	resp, err = bgmAuth(httpc.Put(bgmApiHost()+"/v0/users/-/collections/-/episodes/"+episodeId)).
		Set("Content-Type", "application/json").
		SetBody(body).
		Do()
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// BgmOAuthCallback 使用授权码换取 Token
func BgmOAuthCallback(code string) error {
	cfg := config.Get()
	return bgmOAuthToken(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     cfg.BgmAppID,
		"client_secret": cfg.BgmAppSecret,
		"code":          code,
		"redirect_uri":  cfg.BgmRedirectUri,
	})
}

// bgmOAuthToken 向 BGM 换取 Token 并保存到设置
func bgmOAuthToken(params map[string]string) error {
	body, _ := json.Marshal(params)
	resp, err := httpc.Post("https://bgm.tv/oauth/access_token").
		Set("Content-Type", "application/json").
		SetBody(body).
		Do()
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return err
	}
	out := struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	tx := config.Begin()
	tx.BgmToken = out.AccessToken
	tx.BgmRefreshToken = out.RefreshToken
	tx.Commit()
	return nil
}

// BgmSubjectIdByName 按名称搜索季数一致的条目，优先名称完全一致
func BgmSubjectIdByName(bgmName string, season int) string {
	if strings.TrimSpace(bgmName) == "" {
		return ""
	}
	var list []*entity.BgmInfo
	for _, info := range SearchBgm(bgmName) {
		// 仅保留季数一致
		if info.Season == season {
			list = append(list, info)
		}
	}
	if len(list) == 0 {
		return ""
	}
	// 优先使用名称与季完全匹配的，次之使用第一个
	id := strconv.Itoa(list[0].ID)
	for _, info := range list {
		if info.Name == bgmName || info.NameCn == bgmName {
			id = strconv.Itoa(info.ID)
			break
		}
	}
	time.Sleep(time.Second)
	return id
}

// subscribedBgmIds 已订阅番剧的 BGM ID 集合
func subscribedBgmIds() map[string]bool {
	ids := map[string]bool{}
	for _, ani := range store.ANIList() {
		if id := entity.SubjectIdFromUrl(ani.BgmUrl); id != "" {
			ids[id] = true
		}
	}
	return ids
}
