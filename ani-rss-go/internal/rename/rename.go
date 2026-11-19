// Package rename 重命名算法
package rename

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/utils"
)

// episodeNumReg 从集数片段中提取数字
var episodeNumReg = regexp.MustCompile(`\d+(\.5)?`)

// ReplaceNetworkPlaceholders 替换需要网络查询的占位符：集标题与日文标题（由 service 注入）
var ReplaceNetworkPlaceholders func(template string, ani *entity.Ani, episode float64) string

// Rename 计算资源项的重命名与集数，无法解析集数时返回 false
func Rename(ani *entity.Ani, item *entity.Item) bool {
	offset := ani.Offset
	season := ani.Season
	title := ani.Title

	if ani.GetOva() {
		item.ReName = Del(title, true)
		return true
	}

	customEpisode := utils.Deref(ani.CustomEpisode)
	customEpisodeStr := ani.CustomEpisodeStr
	customEpisodeGroupIndex := ani.CustomEpisodeGroupIndex
	renameTemplate := TemplateOf(ani)

	subgroup := item.Subgroup
	if subgroup == "" {
		subgroup = entity.SubgroupUnknown
	}

	itemTitle := item.Title
	itemTitle = strings.TrimSpace(strings.ReplaceAll(itemTitle, "\n", " "))
	itemTitle = strings.TrimSpace(strings.ReplaceAll(itemTitle, "\t", " "))
	// 去除结尾的 8 位 Hash
	itemTitle = strings.TrimSpace(entity.HashSuffix.ReplaceAllString(itemTitle, ""))

	var e string
	if customEpisode && customEpisodeStr != "" {
		if re, err := regexp.Compile(customEpisodeStr); err == nil {
			if groups := re.FindStringSubmatch(itemTitle); customEpisodeGroupIndex >= 0 && customEpisodeGroupIndex < len(groups) {
				e = groups[customEpisodeGroupIndex]
			}
		}
	} else {
		if groups := entity.EpisodeReg.FindStringSubmatch(itemTitle); len(groups) > 2 {
			e = groups[2]
		}
	}

	if e == "" {
		return false
	}

	episodeStr := episodeNumReg.FindString(e)
	if episodeStr == "" {
		return false
	}

	episode, _ := strconv.ParseFloat(episodeStr, 64)
	episode += float64(offset)
	item.Episode = episode

	seasonFormat := fmt.Sprintf("%02d", season)
	episodeFormat := fmt.Sprintf("%02d", int(episode))
	episodeStr = strconv.Itoa(int(episode))

	// x.5 集
	is5 := episode != float64(int(episode))
	cfg := config.Get()
	if cfg.GetSkip5() && is5 {
		return false
	}
	if is5 {
		episodeFormat += ".5"
		episodeStr += ".5"
	}

	itemTitle = CleanName(itemTitle)
	resolution := resolutionOf(itemTitle)
	tmdbId := ""
	if ani.Tmdb != nil {
		tmdbId = ani.Tmdb.ID
	}

	renameTemplate = strings.ReplaceAll(renameTemplate, "${seasonFormat}", seasonFormat)
	renameTemplate = strings.ReplaceAll(renameTemplate, "${episodeFormat}", episodeFormat)
	renameTemplate = strings.ReplaceAll(renameTemplate, "${season}", strconv.Itoa(season))
	renameTemplate = strings.ReplaceAll(renameTemplate, "${episode}", episodeStr)
	renameTemplate = strings.ReplaceAll(renameTemplate, "${subgroup}", subgroup)
	renameTemplate = strings.ReplaceAll(renameTemplate, "${itemTitle}", itemTitle)
	renameTemplate = strings.ReplaceAll(renameTemplate, "${resolution}", resolution)
	renameTemplate = strings.ReplaceAll(renameTemplate, "${tmdbid}", tmdbId)
	renameTemplate = strings.ReplaceAll(renameTemplate, "${bgmId}", entity.SubjectIdFromUrl(ani.BgmUrl))
	renameTemplate = strings.ReplaceAll(renameTemplate, "${title}", title)

	// 集标题、日文标题需要网络查询，由上层注入
	if ReplaceNetworkPlaceholders != nil {
		renameTemplate = ReplaceNetworkPlaceholders(renameTemplate, ani, episode)
	}

	renameTemplate = strings.ReplaceAll(renameTemplate, "${themoviedbName}", ani.ThemoviedbName)
	renameTemplate = Del(renameTemplate, true)

	reName := CleanName(renameTemplate)

	if maxLen := utils.Deref(cfg.MaxFileNameLength); maxLen > 0 {
		runes := []rune(reName)
		if len(runes) > maxLen {
			reName = string(runes[:maxLen])
		}
	}

	item.ReName = reName
	return true
}

// TemplateOf 获取重命名模板
func TemplateOf(ani *entity.Ani) string {
	t := config.Get().RenameTemplate
	if ani != nil && utils.Deref(ani.CustomRenameTemplateEnable) {
		t = ani.CustomRenameTemplate
	}
	if t == "" {
		t = "${title} S${seasonFormat}E${episodeFormat}"
	}
	return t
}

// resolutionOf 提取分辨率
func resolutionOf(itemTitle string) string {
	itemTitle = strings.ReplaceAll(itemTitle, "1920x1080", "1080p")
	itemTitle = strings.ReplaceAll(itemTitle, "3840x2160", "2160p")
	itemTitle = strings.ReplaceAll(itemTitle, "1280x720", "720p")
	if m := entity.Resolution.FindString(itemTitle); m != "" {
		return strings.ToLower(m)
	}
	return "none"
}

// illegalCharReplacer 文件名非法字符替换
var illegalCharReplacer = strings.NewReplacer(
	"/", " ", "\\", " ", ":", "：", "?", "？", "|", "｜",
	"*", " ", "<", " ", ">", " ", `"`, " ",
)

// CleanName 文件名清洗（非法字符替换）
func CleanName(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "1/2", "½")
	s = illegalCharReplacer.Replace(s)
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// Del 剔除 tmdbid 与年份（遵守设置开关）
func Del(title string, isConfig bool) string {
	if title == "" {
		return ""
	}
	cfg := config.Get()
	if !isConfig {
		title = strings.TrimSpace(entity.TmdbIdReg.ReplaceAllString(title, ""))
		return strings.TrimSpace(entity.YearReg.ReplaceAllString(title, ""))
	}
	if cfg.GetRenameDelTmdbId() {
		title = strings.TrimSpace(entity.TmdbIdReg.ReplaceAllString(title, ""))
	}
	if cfg.GetRenameDelYear() {
		title = strings.TrimSpace(entity.YearReg.ReplaceAllString(title, ""))
	}
	return title
}
