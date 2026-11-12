// Package service — 刮削：NFO、TMDB 图片与 bangumi.ini
package service

import (
	"cmp"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/download"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/items"
	"ani-rss-go/internal/utils"
)

// nfoActor 演员
type nfoActor struct {
	Name   string `xml:"name,omitempty"`
	Role   string `xml:"role,omitempty"`
	Type   string `xml:"type,omitempty"`
	TmdbId string `xml:"tmdbid,omitempty"`
}

// showNfo tvshow.nfo 与电影 nfo
type showNfo struct {
	XMLName       xml.Name
	TmdbId        string     `xml:"tmdbid,omitempty"`
	Title         string     `xml:"title,omitempty"`
	OriginalTitle string     `xml:"originaltitle,omitempty"`
	Year          string     `xml:"year,omitempty"`
	Plot          string     `xml:"plot,omitempty"`
	Rating        string     `xml:"rating,omitempty"`
	Votes         string     `xml:"votes,omitempty"`
	ReleaseDate   string     `xml:"releasedate,omitempty"`
	Tagline       string     `xml:"tagline,omitempty"`
	Runtime       string     `xml:"runtime,omitempty"`
	TmdbEgId      string     `xml:"tmdbegid,omitempty"`
	Genres        []string   `xml:"genre"`
	Actors        []nfoActor `xml:"actor"`
	Trailers      []string   `xml:"trailer"`
	Studios       []string   `xml:"studio"`
}

// seasonNfo season.nfo
type seasonNfo struct {
	XMLName      xml.Name `xml:"season"`
	Title        string   `xml:"title,omitempty"`
	Plot         string   `xml:"plot,omitempty"`
	Outline      string   `xml:"outline,omitempty"`
	SeasonNumber string   `xml:"seasonnumber,omitempty"`
	Year         string   `xml:"year,omitempty"`
	ReleaseDate  string   `xml:"releasedate,omitempty"`
}

// episodeNfo SxxExx.nfo
type episodeNfo struct {
	XMLName xml.Name `xml:"episodedetails"`
	Title   string   `xml:"title,omitempty"`
	Plot    string   `xml:"plot,omitempty"`
	Rating  string   `xml:"rating,omitempty"`
	Year    string   `xml:"year,omitempty"`
	Aired   string   `xml:"aired,omitempty"`
	Episode string   `xml:"episode,omitempty"`
	Season  string   `xml:"season,omitempty"`
	Runtime string   `xml:"runtime,omitempty"`
}

// Scrape 刮削订阅：NFO、图片与 bangumi.ini
func Scrape(ani *entity.Ani, force bool) {
	if ani.Tmdb == nil || ani.Tmdb.ID == "" {
		return
	}
	title := ani.Title
	slog.Info(fmt.Sprintf("正在刮削 ... %s", title))
	downloadPath := download.GetDownloadPath(ani)
	ova := ani.GetOva()
	// 更新tmdb信息
	detail, err := getTmdbDetail(ani.Tmdb, ova)
	if err != nil {
		slog.Warn(fmt.Sprintf("刮削失败: 获取tmdb失败 %s", ani.Tmdb.ID), "error", err)
		return
	}
	if ova {
		err = scrapeMovie(ani.Tmdb, detail, downloadPath, force)
	} else {
		err = scrapeTv(ani.Tmdb, detail, ani.Season, downloadPath, force)
	}
	if err != nil {
		slog.Error(fmt.Sprintf("刮削错误 %s", title), "error", err)
		return
	}
	saveBangumiIni(ani, downloadPath, force)
	slog.Info(fmt.Sprintf("刮削完成 %s", title))
}

// scrapeMovie 电影刮削：以最大的视频文件命名 nfo
func scrapeMovie(tmdb *entity.Tmdb, detail *tmdbDetail, downloadPath string, force bool) error {
	entries, err := os.ReadDir(downloadPath)
	if err != nil {
		return nil
	}
	var video string
	var maxSize int64 = -1
	for _, e := range entries {
		if e.IsDir() || !utils.IsVideoFormat(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Size() > maxSize {
			video, maxSize = e.Name(), info.Size()
		}
	}
	if video == "" {
		// 找不到视频文件
		return nil
	}
	nfoFile := filepath.Join(downloadPath, strings.TrimSuffix(video, filepath.Ext(video))+".nfo")
	if force || !utils.Exists(nfoFile) {
		if err := saveNfo(nfoFile, buildShowNfo(tmdb, detail, "movie")); err != nil {
			return err
		}
	}
	saveTmdbImages(tmdb, detail, true, downloadPath, force)
	return nil
}

// scrapeTv 电视剧刮削：tvshow.nfo、季封面与 season.nfo、每集 thumb 与 nfo
func scrapeTv(tmdb *entity.Tmdb, detail *tmdbDetail, season int, downloadPath string, force bool) error {
	if !utils.Exists(downloadPath) {
		return nil
	}
	showDir := filepath.Dir(downloadPath)
	tvShowNfo := filepath.Join(showDir, "tvshow.nfo")
	if force || !utils.Exists(tvShowNfo) {
		if err := saveNfo(tvShowNfo, buildShowNfo(tmdb, detail, "tvshow")); err != nil {
			return err
		}
	}
	saveTmdbImages(tmdb, detail, false, showDir, force)

	tmdbSeason, err := getTmdbSeason(tmdb, season)
	if err != nil {
		slog.Warn(err.Error())
		return nil
	}
	seasonFormat := fmt.Sprintf("%02d", season)

	// 季封面
	seasonPoster := cmp.Or(tmdbSeason.PosterPath, detail.PosterPath)
	saveTmdbImage(seasonPoster, filepath.Join(showDir, "season"+seasonFormat+"-poster"+filepath.Ext(seasonPoster)), force)

	// 季nfo
	seasonNfoFile := filepath.Join(downloadPath, "season.nfo")
	if force || !utils.Exists(seasonNfoFile) {
		nfo := seasonNfo{
			Title:        tmdbSeason.Name,
			Plot:         tmdbSeason.Overview,
			Outline:      tmdbSeason.Overview,
			SeasonNumber: strconv.Itoa(tmdbSeason.SeasonNumber),
		}
		if t, err := parseDate(tmdbSeason.AirDate); err == nil {
			nfo.Year = strconv.Itoa(t.Year())
			nfo.ReleaseDate = t.Format(time.DateOnly)
		}
		if err := saveNfo(seasonNfoFile, nfo); err != nil {
			return err
		}
	}

	episodeMap := map[int]tmdbEpisode{}
	for _, ep := range tmdbSeason.Episodes {
		episodeMap[ep.EpisodeNumber] = ep
	}

	// 追更天数内播出的集强制刷新元数据
	followDay := utils.Deref(config.Get().FollowDay)
	followSince := time.Now().AddDate(0, 0, -followDay)

	entries, err := os.ReadDir(downloadPath)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !utils.IsVideoFormat(name) {
			continue
		}
		mainName := strings.TrimSuffix(name, filepath.Ext(name))
		m := entity.SeasonReg.FindStringSubmatch(mainName)
		if m == nil {
			// 命名不标准
			continue
		}
		if seasonNumber, _ := strconv.Atoi(m[1]); seasonNumber != season {
			// 季对应不上 跳过
			continue
		}
		episodeNumber, _ := strconv.ParseFloat(m[2], 64)
		if items.Is5(episodeNumber) {
			// x.5 集没有对应的 TMDB 集
			continue
		}
		ep, exists := episodeMap[int(episodeNumber)]
		if !exists {
			// 找不到对应集
			continue
		}
		airDate, err := parseDate(ep.AirDate)
		if err != nil {
			airDate = time.Now()
		}
		follow := !airDate.Before(followSince)

		if ep.StillPath != "" {
			saveTmdbImage(ep.StillPath, filepath.Join(downloadPath, mainName+"-thumb"+filepath.Ext(ep.StillPath)), follow || force)
		}

		episodeFile := filepath.Join(downloadPath, mainName+".nfo")
		if follow || force || !utils.Exists(episodeFile) {
			nfo := episodeNfo{
				Title:   ep.Name,
				Plot:    ep.Overview,
				Rating:  strconv.FormatFloat(ep.VoteAverage, 'f', -1, 64),
				Episode: strconv.Itoa(ep.EpisodeNumber),
				Season:  strconv.Itoa(ep.SeasonNumber),
			}
			if err == nil {
				nfo.Year = strconv.Itoa(airDate.Year())
				nfo.Aired = airDate.Format(time.DateOnly)
			}
			if ep.Runtime > 0 {
				nfo.Runtime = strconv.Itoa(ep.Runtime)
			}
			if err := saveNfo(episodeFile, nfo); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildShowNfo 生成 tvshow.nfo 或电影 nfo 内容
func buildShowNfo(tmdb *entity.Tmdb, detail *tmdbDetail, rootTag string) showNfo {
	nfo := showNfo{
		XMLName:       xml.Name{Local: rootTag},
		TmdbId:        strconv.FormatInt(detail.ID, 10),
		Title:         detail.Name,
		OriginalTitle: detail.OriginalName,
		Plot:          detail.Overview,
		Rating:        strconv.FormatFloat(detail.VoteAverage, 'f', -1, 64),
		Votes:         strconv.Itoa(detail.VoteCount),
		Tagline:       detail.Tagline,
		TmdbEgId:      tmdb.GroupID,
	}
	if t, err := parseDate(detail.FirstAirDate); err == nil {
		nfo.Year = strconv.Itoa(t.Year())
		nfo.ReleaseDate = t.Format(time.DateOnly)
	}
	if detail.Runtime > 0 {
		nfo.Runtime = strconv.Itoa(detail.Runtime)
	}
	for _, g := range detail.Genres {
		nfo.Genres = append(nfo.Genres, g.Name)
	}
	for _, c := range detail.Credits.Cast {
		nfo.Actors = append(nfo.Actors, nfoActor{Name: c.Name, Role: c.Character, Type: "Actor", TmdbId: strconv.FormatInt(c.ID, 10)})
	}
	for _, v := range detail.Videos.Results {
		if v.Site == "YouTube" {
			nfo.Trailers = append(nfo.Trailers, "https://www.youtube.com/watch?v="+v.Key)
		}
	}
	for _, n := range detail.Networks {
		nfo.Studios = append(nfo.Studios, n.Name)
	}
	return nfo
}

// saveNfo 以 4 空格缩进写入 XML
func saveNfo(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := xml.MarshalIndent(v, "", "    ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append([]byte(xml.Header), data...), 0o644); err != nil {
		return err
	}
	slog.Info(fmt.Sprintf("已保存NFO %s", path))
	return nil
}

// saveTmdbImages 保存封面、背景图、最多 4 张额外背景图与 logo
func saveTmdbImages(tmdb *entity.Tmdb, detail *tmdbDetail, ova bool, outputPath string, force bool) {
	saveTmdbImage(detail.PosterPath, filepath.Join(outputPath, "poster"+filepath.Ext(detail.PosterPath)), force)
	saveTmdbImage(detail.BackdropPath, filepath.Join(outputPath, "fanart"+filepath.Ext(detail.BackdropPath)), force)

	images, err := getTmdbImages(tmdb, ova)
	if err != nil {
		slog.Warn(err.Error())
		return
	}
	n := 0
	for _, img := range images.Backdrops {
		if n >= 4 {
			break
		}
		if img.FilePath == detail.BackdropPath || img.Width < 1280 {
			continue
		}
		n++
		saveTmdbImage(img.FilePath, filepath.Join(outputPath, fmt.Sprintf("fanart%d%s", n, filepath.Ext(img.FilePath))), force)
	}
	if len(images.Logos) > 0 {
		logo := images.Logos[0].FilePath
		saveTmdbImage(logo, filepath.Join(outputPath, "clearlogo"+filepath.Ext(logo)), force)
	}
}

// saveTmdbImage 下载 TMDB 原图，非强制时已存在则跳过
func saveTmdbImage(tmdbPath, saveFile string, force bool) {
	if tmdbPath == "" {
		return
	}
	if !force && utils.Exists(saveFile) {
		return
	}
	host := cmp.Or(strings.TrimRight(config.Get().TmdbImage, "/"), "https://image.tmdb.org")
	resp, err := httpc.Get(host + "/t/p/original" + tmdbPath).Do()
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		slog.Warn(err.Error())
		return
	}
	tmp := saveFile + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err == nil {
		err = os.Rename(tmp, saveFile)
	}
	if err != nil {
		os.Remove(tmp)
		slog.Error(err.Error())
		return
	}
	slog.Info(fmt.Sprintf("已保存图片 %s", saveFile))
}

// saveBangumiIni 保存 bangumi.ini
func saveBangumiIni(ani *entity.Ani, downloadPath string, force bool) {
	cfg := config.Get()
	if !utils.Deref(cfg.BangumiIniEnabled) {
		// 未开启 bangumi.ini
		return
	}
	file := filepath.Join(downloadPath, "bangumi.ini")
	if !force && utils.Exists(file) {
		// 非强制模式
		return
	}
	content := fmt.Sprintf("[Bangumi]\nid=%s\noffset=%d\n", GetSubjectId(ani), ani.Offset)
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		slog.Error(err.Error())
		return
	}
	slog.Info(fmt.Sprintf("已保存 %s", file))
}
