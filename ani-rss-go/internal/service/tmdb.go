package service

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/rename"
	"ani-rss-go/internal/utils"
)

type tmdbRecord struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	OriginalName  string `json:"original_name"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	ReleaseDate   string `json:"release_date"`
	FirstAirDate  string `json:"first_air_date"`
	MediaType     string `json:"media_type"`
}

type tmdbSearchResponse struct {
	Results []tmdbRecord `json:"results"`
}

type tmdbGroupsResponse struct {
	Results []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		Type         int    `json:"type"`
		GroupCount   int    `json:"group_count"`
		EpisodeCount int    `json:"episode_count"`
	} `json:"results"`
}

func GetTMDBName(in *entity.TMDBNameRequest) (*entity.TMDBNameResponse, error) {
	if in == nil || (strings.TrimSpace(in.TMDBID) == "" && strings.TrimSpace(in.Title) == "") {
		return nil, errors.New("TmdbId 或 标题 不能为空")
	}
	cfg := config.Get()
	mediaType := tmdbMediaType(in.Ova)
	var tmdb *entity.Tmdb
	if strings.TrimSpace(in.TMDBID) != "" {
		id, err := validTMDBID(in.TMDBID)
		if err != nil {
			return nil, err
		}
		record, err := tmdbJSON[tmdbRecord](cfg, mediaType+"/"+id, nil)
		if err != nil {
			return nil, err
		}
		tmdb = record.toEntity()
	} else {
		title := strings.TrimSpace(rename.Del(in.Title, false))
		if title == "" {
			return nil, errors.New("标题不能为空")
		}
		searchType := mediaType
		if mediaType == "tv" && !utils.DerefOr(cfg.TmdbAnime, true) {
			searchType = "multi"
		}
		var err error
		tmdb, err = searchTMDBFirst(cfg, title, searchType)
		if err != nil {
			return nil, err
		}
	}
	if tmdb == nil {
		return nil, errors.New("未获取到 TMDB")
	}
	name := TmdbFinalNameOf(tmdb)
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("获取 TMDB 失败")
	}
	return &entity.TMDBNameResponse{ThemoviedbName: name, TMDB: tmdb}, nil
}

func GetTMDBGroups(ani *entity.Ani) ([]entity.TMDBGroup, error) {
	if ani == nil || ani.Tmdb == nil || strings.TrimSpace(ani.Tmdb.ID) == "" {
		return nil, errors.New("TMDB 不能为空")
	}
	id, err := validTMDBID(ani.Tmdb.ID)
	if err != nil {
		return nil, err
	}
	out, err := tmdbJSON[tmdbGroupsResponse](config.Get(), "tv/"+id+"/episode_groups", nil)
	if err != nil {
		return nil, err
	}
	if out.Results == nil {
		return nil, errors.New("TMDB 响应缺少剧集组列表")
	}
	typeNames := [...]string{"未知", "原始播出日期", "绝对顺序", "DVD", "数字发行", "故事篇章", "制作顺序", "电视播出"}
	groups := make([]entity.TMDBGroup, 0, len(out.Results))
	for _, group := range out.Results {
		if group.ID == "" {
			continue
		}
		typeName := typeNames[0]
		if group.Type > 0 && group.Type < len(typeNames) {
			typeName = typeNames[group.Type]
		}
		groups = append(groups, entity.TMDBGroup{
			ID:           group.ID,
			Name:         group.Name,
			Description:  group.Description,
			Type:         group.Type,
			TypeName:     typeName,
			GroupCount:   group.GroupCount,
			EpisodeCount: group.EpisodeCount,
		})
	}
	return groups, nil
}

func validTMDBID(value string) (string, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(value), 10, 63)
	if err != nil || id == 0 {
		return "", errors.New("TMDB ID 无效")
	}
	return strconv.FormatUint(id, 10), nil
}

func (r tmdbRecord) toEntity() *entity.Tmdb {
	name, originalName, date := r.Name, r.OriginalName, r.FirstAirDate
	if name == "" {
		name, originalName, date = r.Title, r.OriginalTitle, r.ReleaseDate
	}
	if r.ID <= 0 || strings.TrimSpace(name) == "" {
		return nil
	}
	tmdb := &entity.Tmdb{ID: strconv.FormatInt(r.ID, 10), Name: name, OriginalName: originalName}
	if t, err := parseDate(date); date != "" && err == nil {
		tmdb.Date = &entity.Date{Time: t}
	}
	return tmdb
}

// searchTMDB 搜索 TMDB，返回有效结果，MediaType 为该结果实际的 tv 或 movie
func searchTMDB(cfg *entity.Config, title, mediaType string) ([]tmdbRecord, error) {
	out, err := tmdbJSON[tmdbSearchResponse](cfg, "search/"+mediaType, url.Values{"query": {title}})
	if err != nil {
		return nil, err
	}
	if out.Results == nil {
		return nil, errors.New("TMDB 响应缺少搜索结果")
	}
	list := make([]tmdbRecord, 0, len(out.Results))
	for _, record := range out.Results {
		if mediaType == "multi" && record.MediaType != "tv" && record.MediaType != "movie" {
			continue
		}
		if record.toEntity() == nil {
			continue
		}
		if mediaType != "multi" {
			record.MediaType = mediaType
		}
		list = append(list, record)
	}
	return list, nil
}

// searchTMDBFirst 搜索 TMDB 取首个结果，开启罗马音时替换名称
func searchTMDBFirst(cfg *entity.Config, title, mediaType string) (*entity.Tmdb, error) {
	list, err := searchTMDB(cfg, title, mediaType)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	tmdb := list[0].toEntity()
	tmdbRomaji(cfg, tmdb, list[0].MediaType)
	return tmdb, nil
}

// tmdbAlternativeTitles TMDB 别名列表，剧集返回 results，电影返回 titles
type tmdbAlternativeTitles struct {
	Results []tmdbAlternativeTitle `json:"results"`
	Titles  []tmdbAlternativeTitle `json:"titles"`
}

type tmdbAlternativeTitle struct {
	Iso31661 string `json:"iso_3166_1"`
	Title    string `json:"title"`
	Type     string `json:"type"`
}

// tmdbRomaji 开启罗马音时优先使用 TMDB 日本罗马音别名，其次通过 AniList 获取
func tmdbRomaji(cfg *entity.Config, tmdb *entity.Tmdb, mediaType string) {
	if tmdb == nil || !utils.Deref(cfg.TmdbRomaji) {
		return
	}
	out, err := tmdbJSON[tmdbAlternativeTitles](cfg, mediaType+"/"+tmdb.ID+"/alternative_titles", nil)
	if err != nil {
		slog.Error("获取 TMDB 别名失败", "tmdbId", tmdb.ID, "error", err)
	}
	for _, alt := range append(out.Results, out.Titles...) {
		if alt.Iso31661 != "JP" {
			continue
		}
		altType := strings.ToLower(alt.Type)
		if altType != "romaji" && altType != "romanization" {
			continue
		}
		if title := rename.CleanName(alt.Title); title != "" {
			tmdb.Name = title
			return
		}
	}
	romaji, err := aniListRomaji(tmdb.Name)
	if err != nil {
		slog.Error("通过AniList获取罗马音失败", "error", err)
		return
	}
	if romaji = rename.CleanName(romaji); romaji != "" {
		tmdb.Name = romaji
	}
}

func tmdbJSON[T any](cfg *entity.Config, path string, query url.Values) (T, error) {
	var out T
	host := cmp.Or(strings.TrimRight(strings.TrimSpace(cfg.TmdbApi), "/"), "https://api.themoviedb.org")
	u, err := url.Parse(host)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" {
		return out, errors.New("TMDB API 地址无效")
	}
	req := httpc.Get(host+"/3/"+path).AddQuery("api_key", tmdbApiKeyOf(cfg))
	if cfg.TmdbLanguage != "" {
		req.AddQuery("language", cfg.TmdbLanguage)
	}
	for key, values := range query {
		for _, value := range values {
			req.AddQuery(key, value)
		}
	}
	resp, err := req.Do()
	if err != nil {
		return out, fmt.Errorf("TMDB 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("TMDB 请求失败，HTTP 状态码为 %d", resp.StatusCode)
	}
	const maxResponseBytes = 4 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return out, fmt.Errorf("读取 TMDB 响应失败: %w", err)
	}
	if len(data) > maxResponseBytes {
		return out, errors.New("TMDB 响应超过 4 MiB")
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("TMDB 响应格式无效: %w", err)
	}
	return out, nil
}

// tmdbDetail TMDB 剧集或电影详情
type tmdbDetail struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	OriginalName   string  `json:"original_name"`
	Title          string  `json:"title"`
	OriginalTitle  string  `json:"original_title"`
	FirstAirDate   string  `json:"first_air_date"`
	ReleaseDate    string  `json:"release_date"`
	Overview       string  `json:"overview"`
	Tagline        string  `json:"tagline"`
	VoteAverage    float64 `json:"vote_average"`
	VoteCount      int     `json:"vote_count"`
	Runtime        int     `json:"runtime"`
	EpisodeRunTime []int   `json:"episode_run_time"`
	PosterPath     string  `json:"poster_path"`
	BackdropPath   string  `json:"backdrop_path"`
	Genres         []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Networks []struct {
		Name string `json:"name"`
	} `json:"networks"`
	ProductionCompanies []struct {
		Name string `json:"name"`
	} `json:"production_companies"`
	Credits struct {
		Cast []struct {
			ID        int64  `json:"id"`
			Name      string `json:"name"`
			Character string `json:"character"`
		} `json:"cast"`
	} `json:"credits"`
	Videos struct {
		Results []struct {
			Site string `json:"site"`
			Key  string `json:"key"`
		} `json:"results"`
	} `json:"videos"`
}

// tmdbEpisode TMDB 单集
type tmdbEpisode struct {
	EpisodeNumber int     `json:"episode_number"`
	SeasonNumber  int     `json:"season_number"`
	Order         int     `json:"order"`
	Name          string  `json:"name"`
	Overview      string  `json:"overview"`
	VoteAverage   float64 `json:"vote_average"`
	AirDate       string  `json:"air_date"`
	Runtime       int     `json:"runtime"`
	StillPath     string  `json:"still_path"`
}

// tmdbSeason TMDB 季
type tmdbSeason struct {
	Name         string        `json:"name"`
	Overview     string        `json:"overview"`
	SeasonNumber int           `json:"season_number"`
	AirDate      string        `json:"air_date"`
	PosterPath   string        `json:"poster_path"`
	Episodes     []tmdbEpisode `json:"episodes"`
}

// tmdbImages TMDB 图片列表
type tmdbImages struct {
	Backdrops []tmdbImage `json:"backdrops"`
	Logos     []tmdbImage `json:"logos"`
}

type tmdbImage struct {
	FilePath string `json:"file_path"`
	Width    int    `json:"width"`
}

// tmdbMediaType 剧场版使用 movie，其余使用 tv
func tmdbMediaType(ova bool) string {
	if ova {
		return "movie"
	}
	return "tv"
}

// getTmdbDetail 获取 TMDB 详情，附带演职人员与视频
func getTmdbDetail(tmdb *entity.Tmdb, ova bool) (*tmdbDetail, error) {
	id, err := validTMDBID(tmdb.ID)
	if err != nil {
		return nil, err
	}
	detail, err := tmdbJSON[tmdbDetail](config.Get(), tmdbMediaType(ova)+"/"+id, url.Values{"append_to_response": {"credits,videos"}})
	if err != nil {
		return nil, err
	}
	if detail.Name == "" {
		detail.Name, detail.OriginalName, detail.FirstAirDate = detail.Title, detail.OriginalTitle, detail.ReleaseDate
	}
	if detail.Runtime == 0 && len(detail.EpisodeRunTime) > 0 {
		detail.Runtime = detail.EpisodeRunTime[0]
	}
	if len(detail.Networks) == 0 {
		detail.Networks = detail.ProductionCompanies
	}
	return &detail, nil
}

// getTmdbSeason 获取季信息，设置剧集组时取该组中对应顺序的分组并按组内顺序编号
func getTmdbSeason(tmdb *entity.Tmdb, season int) (*tmdbSeason, error) {
	id, err := validTMDBID(tmdb.ID)
	if err != nil {
		return nil, err
	}
	cfg := config.Get()
	if strings.TrimSpace(tmdb.GroupID) == "" {
		out, err := tmdbJSON[tmdbSeason](cfg, "tv/"+id+"/season/"+strconv.Itoa(season), nil)
		if err != nil {
			return nil, err
		}
		return &out, nil
	}

	group, err := tmdbJSON[struct {
		Groups []struct {
			Name     string        `json:"name"`
			Order    int           `json:"order"`
			Episodes []tmdbEpisode `json:"episodes"`
		} `json:"groups"`
	}](cfg, "tv/episode_group/"+url.PathEscape(strings.TrimSpace(tmdb.GroupID)), nil)
	if err != nil {
		return nil, err
	}
	for _, g := range group.Groups {
		if g.Order != season {
			continue
		}
		out := &tmdbSeason{Name: g.Name, SeasonNumber: season, Episodes: g.Episodes}
		for i := range out.Episodes {
			out.Episodes[i].EpisodeNumber = out.Episodes[i].Order + 1
			out.Episodes[i].SeasonNumber = season
		}
		if len(out.Episodes) > 0 {
			out.AirDate = out.Episodes[0].AirDate
		}
		return out, nil
	}
	return nil, fmt.Errorf("剧集组 %s 中不存在第 %d 季", tmdb.GroupID, season)
}

// getTmdbImages 获取背景图与 logo，优先当前语言，其次无语言与英文
func getTmdbImages(tmdb *entity.Tmdb, ova bool) (*tmdbImages, error) {
	id, err := validTMDBID(tmdb.ID)
	if err != nil {
		return nil, err
	}
	cfg := config.Get()
	langs := []string{"null", "en"}
	if lang, _, _ := strings.Cut(cfg.TmdbLanguage, "-"); lang != "" && lang != "en" {
		langs = append([]string{lang}, langs...)
	}
	out, err := tmdbJSON[tmdbImages](cfg, tmdbMediaType(ova)+"/"+id+"/images", url.Values{"include_image_language": {strings.Join(langs, ",")}})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

const fallbackTmdbApiKey = "450e4f651e1c93e31383e20f8e731e5f"

// tmdbApiKeyOf 用户未配置时回退到内置 API Key
func tmdbApiKeyOf(cfg *entity.Config) string {
	return cmp.Or(cfg.TmdbApiKey, fallbackTmdbApiKey)
}

// TmdbFinalName 获取番剧在 tmdb 的名称（网络搜索并回填 ani.Tmdb）
func TmdbFinalName(ani *entity.Ani) string {
	name := rename.Del(ani.Title, false)
	if strings.TrimSpace(name) == "" {
		return ""
	}
	cfg := config.Get()
	// 剧场版搜索电影
	tmdbType := "movie"
	if !ani.GetOva() {
		tmdbType = "tv"
		if !utils.DerefOr(cfg.TmdbAnime, true) {
			tmdbType = "multi"
		}
	}
	tmdb, _ := searchTMDBFirst(cfg, name, tmdbType)
	if tmdb == nil {
		return ""
	}
	ani.Tmdb = tmdb
	return TmdbFinalNameOf(tmdb)
}

// TmdbFinalNameOf 获取添加年份与 tmdbid 后的名称
func TmdbFinalNameOf(tmdb *entity.Tmdb) string {
	cfg := config.Get()
	name := tmdb.Name
	if utils.Deref(cfg.TmdbOriginalName) && tmdb.OriginalName != "" {
		name = tmdb.OriginalName
	}
	return TmdbFinalNameTitled(name, tmdb)
}

// TmdbFinalNameTitled 为标题追加年份与 tmdbid 并清洗
func TmdbFinalNameTitled(title string, tmdb *entity.Tmdb) string {
	if tmdb == nil {
		return title
	}
	cfg := config.Get()
	if utils.Deref(cfg.TitleYear) {
		title = rename.Del(title, false)
		if tmdb.Date != nil {
			title = fmt.Sprintf("%s (%d)", title, tmdb.Date.Year())
		}
	}
	if utils.Deref(cfg.TmdbId) {
		if utils.Deref(cfg.TmdbIdPlexMode) {
			title = fmt.Sprintf("%s {tmdb-%s}", title, tmdb.ID)
		} else {
			title = fmt.Sprintf("%s [tmdbid=%s]", title, tmdb.ID)
		}
	}
	return rename.CleanName(title)
}

// TmdbEpisodeTitleMap 获取 TMDB 每集标题，支持剧集组
func TmdbEpisodeTitleMap(ani *entity.Ani) map[int]string {
	if ani == nil || ani.GetOva() || ani.Tmdb == nil || ani.Tmdb.ID == "" {
		return nil
	}
	key := fmt.Sprintf("TMDB_getEpisodeTitleMap:%s:%s:%d", ani.Tmdb.ID, ani.Tmdb.GroupID, ani.Season)
	if v, ok := utils.CacheGet[map[int]string](key); ok {
		return v
	}
	m := map[int]string{}
	season, err := getTmdbSeason(ani.Tmdb, ani.Season)
	if err != nil {
		slog.Error(err.Error())
	} else {
		for _, ep := range season.Episodes {
			m[ep.EpisodeNumber] = ep.Name
		}
	}
	if len(m) == 0 {
		utils.Cache.Set(key, m, 10*time.Second)
	} else {
		utils.Cache.Set(key, m, 5*time.Minute)
	}
	return m
}

// parseDate 解析 TMDB 返回的日期
func parseDate(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, &time.ParseError{Value: s, Layout: "date"}
}
