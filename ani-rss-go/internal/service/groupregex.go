// Package service — 字幕组标签与匹配规则
package service

import (
	"regexp"
	"slices"

	"ani-rss-go/internal/entity"
)

// groupTagRegexes 字幕组标签正则
var groupTagRegexes = []*regexp.Regexp{
	regexp.MustCompile(`1920[Xx]1080`),
	regexp.MustCompile(`3840[Xx]2160`),
	regexp.MustCompile(`1080[Pp]`),
	regexp.MustCompile(`720[Pp]`),
	regexp.MustCompile(`4[Kk]`),
	regexp.MustCompile(`繁`),
	regexp.MustCompile(`简`),
	regexp.MustCompile(`日`),
	regexp.MustCompile(`内嵌`),
	regexp.MustCompile(`内封`),
	regexp.MustCompile(`外挂`),
	regexp.MustCompile(`cht|Cht|CHT`),
	regexp.MustCompile(`chs|Chs|CHS`),
	regexp.MustCompile(`avc|Avc|AVC`),
	regexp.MustCompile(`hevc|Hevc|HEVC`),
	regexp.MustCompile(`h264|H264`),
	regexp.MustCompile(`h265|H265`),
	regexp.MustCompile(`10bit|10Bit|10BIT`),
	regexp.MustCompile(`mp4|MP4`),
	regexp.MustCompile(`mkv|MKV`),
}

// toGroupRegex 根据标题列表计算字幕组标签
func toGroupRegex(titles []string) entity.GroupRegex {
	regexList := [][]entity.GroupRegexItem{}
	tags := []string{}
	seenTitle := map[string]bool{}
	for _, title := range titles {
		if seenTitle[title] {
			continue
		}
		seenTitle[title] = true
		seenTag := map[string]bool{}
		var row []entity.GroupRegexItem
		for _, re := range groupTagRegexes {
			tag := re.FindString(title)
			if tag == "" {
				continue
			}
			if seenTag[re.String()] {
				continue
			}
			seenTag[re.String()] = true
			row = append(row, entity.GroupRegexItem{Label: tag, Regex: re.String()})
			if len(tags) < 5 && !slices.Contains(tags, tag) {
				tags = append(tags, tag)
			}
		}
		if len(row) == 0 {
			continue
		}
		if !slices.ContainsFunc(regexList, func(prev []entity.GroupRegexItem) bool { return slices.Equal(prev, row) }) {
			regexList = append(regexList, row)
		}
	}
	return entity.GroupRegex{RegexList: regexList, Tags: tags}
}
