// Package service — 星期标签与排序
package service

import (
	"math"
	"regexp"
	"time"
)

// weekLabels 星期日至星期六的标签，下标与 time.Weekday 对应
var weekLabels = [7]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}

// weekLabelRes 星期日至星期六的匹配规则，兼容「星期一」与「周一」
var weekLabelRes = [7]*regexp.Regexp{
	regexp.MustCompile(`(星期|周)日`), regexp.MustCompile(`(星期|周)一`), regexp.MustCompile(`(星期|周)二`),
	regexp.MustCompile(`(星期|周)三`), regexp.MustCompile(`(星期|周)四`), regexp.MustCompile(`(星期|周)五`),
	regexp.MustCompile(`(星期|周)六`),
}

// weekOrder 星期标签的排序位置：今天为 0，之后按时间倒序递增，无法识别的标签排在最后
func weekOrder(label string) int {
	today := int(time.Now().Weekday())
	for i := range 7 {
		if weekLabelRes[(today-i+7)%7].MatchString(label) {
			return i
		}
	}
	return math.MaxInt
}
