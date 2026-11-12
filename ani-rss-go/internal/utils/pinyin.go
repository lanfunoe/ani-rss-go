package utils

import (
	"strings"

	"github.com/mozillazg/go-pinyin"
)

var pinyinArgs = pinyin.NewArgs()

// pinyinPhrases 双字多音字词组的读音，优先于单字读音
var pinyinPhrases = map[string][2]string{
	"重庆": {"chong", "qing"},
	"重启": {"chong", "qi"},
	"重回": {"chong", "hui"},
	"重生": {"chong", "sheng"},
	"重来": {"chong", "lai"},
	"调教": {"tiao", "jiao"},
}

// pinyinReading 单个非 ASCII 字符的首个读音，ASCII 字符或无拼音时返回空串
func pinyinReading(r rune) string {
	if r < 128 {
		return ""
	}
	if pys := pinyin.SinglePinyin(r, pinyinArgs); len(pys) > 0 {
		return pys[0]
	}
	return ""
}

// pinyinReadings 逐字读音，命中多音字词组时使用词组读音，ASCII 字符或无拼音时为空串
func pinyinReadings(s string) ([]rune, []string) {
	runes := []rune(strings.TrimSpace(s))
	pys := make([]string, len(runes))
	for i := 0; i < len(runes); i++ {
		if i+1 < len(runes) {
			if phrase, ok := pinyinPhrases[string(runes[i:i+2])]; ok {
				pys[i], pys[i+1] = phrase[0], phrase[1]
				i++
				continue
			}
		}
		pys[i] = pinyinReading(runes[i])
	}
	return runes, pys
}

// GetPinyin 全拼，无拼音的字符原样保留（TinyPinyin 行为）
func GetPinyin(s string) string {
	var b strings.Builder
	runes, pys := pinyinReadings(s)
	for i, r := range runes {
		if pys[i] != "" {
			b.WriteString(pys[i])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// GetFirstLetter 拼音首字母，无拼音的字符原样保留（TinyPinyin 行为）
func GetFirstLetter(s string) string {
	var b strings.Builder
	runes, pys := pinyinReadings(s)
	for i, r := range runes {
		if pys[i] != "" {
			b.WriteString(strings.ToUpper(pys[i][:1]))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// GetPinyinInitialLetters 首字母分类：数字归为 0，非字母归为 #
func GetPinyinInitialLetters(s string) string {
	for _, r := range GetPinyin(s) {
		switch {
		case r >= '0' && r <= '9':
			return "0"
		case r >= 'a' && r <= 'z':
			return strings.ToUpper(string(r))
		case r >= 'A' && r <= 'Z':
			return string(r)
		default:
			return "#"
		}
	}
	return "0"
}
