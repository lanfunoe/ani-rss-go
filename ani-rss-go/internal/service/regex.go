package service

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/utils"
)

// checkRegex 编译正则，失败时返回带中文提示的错误。
// Go 使用 RE2 引擎，不支持 Java 正则中的前瞻、后顾、反向引用等语法。
func checkRegex(s string) error {
	_, err := regexp.Compile(s)
	if err == nil {
		return nil
	}
	se, ok := errors.AsType[*syntax.Error](err)
	if !ok {
		return err
	}
	switch expr := se.Expr; {
	case strings.HasPrefix(expr, "(?=") || strings.HasPrefix(expr, "(?!"):
		return errors.New("Go 不支持前瞻语法 (?=…)、(?!…)")
	case strings.HasPrefix(expr, "(?<=") || strings.HasPrefix(expr, "(?<!"):
		return errors.New("Go 不支持后顾语法 (?<=…)、(?<!…)")
	case strings.HasPrefix(expr, "(?>"):
		return errors.New("Go 不支持原子组 (?>…)")
	case se.Code == syntax.ErrInvalidEscape && len(expr) == 2 && expr[1] >= '1' && expr[1] <= '9':
		return fmt.Errorf("Go 不支持反向引用 %s", expr)
	case se.Code == syntax.ErrInvalidRepeatOp && strings.HasSuffix(expr, "+"):
		return errors.New("Go 不支持占有量词 *+、++、?+")
	}
	return err
}

// checkRules 校验匹配/排除规则列表，{{字幕组}}:规则 只校验冒号后的部分
func checkRules(kind string, rules []string) error {
	for _, r := range rules {
		body := r
		if m := entity.SubgroupRe.FindStringSubmatch(r); m != nil {
			body = m[2]
		}
		if err := checkRegex(body); err != nil {
			return fmt.Errorf("%s「%s」无效：%w", kind, r, err)
		}
	}
	return nil
}

// validateAniRegex 校验订阅中的匹配、排除与自定义集数正则
func validateAniRegex(ani *entity.Ani) error {
	if err := checkRules("匹配规则", ani.Match); err != nil {
		return err
	}
	if err := checkRules("排除规则", ani.Exclude); err != nil {
		return err
	}
	if utils.Deref(ani.CustomEpisode) && ani.CustomEpisodeStr != "" {
		if err := checkRegex(ani.CustomEpisodeStr); err != nil {
			return fmt.Errorf("自定义集数正则「%s」无效：%w", ani.CustomEpisodeStr, err)
		}
	}
	return nil
}
