// 下载位置模板解析
package download

import (
	"fmt"
	"strconv"
	"strings"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/utils"
)

// GetDownloadPath 计算下载位置
func GetDownloadPath(ani *entity.Ani) string {
	return GetDownloadPathByTemplate(ani, "")
}

// GetDownloadPathByTemplate 使用指定模板计算下载位置
func GetDownloadPathByTemplate(ani *entity.Ani, templateOverride string) string {
	cfg := config.Get()
	template := cfg.DownloadPathTemplate
	if ani.GetOva() && cfg.OvaDownloadPathTemplate != "" {
		template = cfg.OvaDownloadPathTemplate
	}
	if templateOverride != "" {
		template = templateOverride
	} else if ani.GetCustomDownloadPath() && ani.CustomDownloadPathTemplate != "" {
		for line := range strings.SplitSeq(ani.CustomDownloadPathTemplate, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				template = line
				break
			}
		}
	}

	title := strings.TrimSpace(ani.Title)
	letter := utils.GetPinyinInitialLetters(title)
	template = strings.ReplaceAll(template, "${letter}", letter)

	rd := ani.ReleaseDate
	year, month := rd.Year(), int(rd.Month())
	monthFormat := fmt.Sprintf("%02d", month)
	// TMDB 首播年份，未获取到 TMDB 日期时使用订阅的首播年份
	tmdbYear := year
	if ani.Tmdb != nil && ani.Tmdb.Date != nil && !ani.Tmdb.Date.IsZero() {
		tmdbYear = ani.Tmdb.Date.Year()
	}

	if strings.Contains(template, "${quarter}") ||
		strings.Contains(template, "${quarterFormat}") ||
		strings.Contains(template, "${quarterName}") {
		var quarter int
		var quarterName string
		switch {
		case month == 12 || month == 1 || month == 2:
			if month == 12 {
				year++
			}
			quarter, quarterName = 1, "冬"
		case month >= 3 && month <= 5:
			quarter, quarterName = 4, "春"
		case month >= 6 && month <= 8:
			quarter, quarterName = 7, "夏"
		default:
			quarter, quarterName = 10, "秋"
		}
		template = strings.ReplaceAll(template, "${quarter}", strconv.Itoa(quarter))
		template = strings.ReplaceAll(template, "${quarterFormat}", fmt.Sprintf("%02d", quarter))
		template = strings.ReplaceAll(template, "${quarterName}", quarterName)
	}

	template = strings.ReplaceAll(template, "${tmdbYear}", strconv.Itoa(tmdbYear))
	template = strings.ReplaceAll(template, "${year}", strconv.Itoa(year))
	template = strings.ReplaceAll(template, "${month}", strconv.Itoa(month))
	template = strings.ReplaceAll(template, "${monthFormat}", monthFormat)
	template = strings.ReplaceAll(template, "${season}", strconv.Itoa(ani.Season))
	template = strings.ReplaceAll(template, "${seasonFormat}", fmt.Sprintf("%02d", ani.Season))
	template = strings.ReplaceAll(template, "${title}", title)
	template = strings.ReplaceAll(template, "${themoviedbName}", ani.ThemoviedbName)
	template = strings.ReplaceAll(template, "${subgroup}", ani.Subgroup)

	tmdbId := ""
	if ani.Tmdb != nil {
		tmdbId = ani.Tmdb.ID
	}
	template = strings.ReplaceAll(template, "${tmdbid}", tmdbId)
	template = strings.ReplaceAll(template, "${bgmId}", entity.SubjectIdFromUrl(ani.BgmUrl))

	jpTitle := ani.JpTitle
	if strings.Contains(template, "${jpTitle}") && jpTitle == "" {
		jpTitle = ani.Title
	}
	template = strings.ReplaceAll(template, "${jpTitle}", jpTitle)

	return utils.AbsPath(template)
}
