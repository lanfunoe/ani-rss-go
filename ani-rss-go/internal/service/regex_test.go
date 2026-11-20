package service

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
)

// TestMain 将配置目录指向临时目录，防止测试写入真实配置
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ani-rss-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("CONFIG", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// wantErr 断言 err 为 nil（want 为空）或包含 want
func wantErr(t *testing.T, name string, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Errorf("%s: got %v, want nil", name, err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("%s: got %v, want error containing %q", name, err, want)
	}
}

func TestCheckRegex(t *testing.T) {
	cases := []struct {
		re   string
		want string
	}{
		{`1080[Pp]`, ""},
		{`(?<name>\d+)`, ""},
		{`\p{Han}`, ""},
		{`(?i)abc`, ""},
		{`a*?`, ""},
		{`^(?!.*合集).*1080p`, "Go 不支持前瞻语法"},
		{`(?=abc)`, "Go 不支持前瞻语法"},
		{`(?<=\[)\d+`, "Go 不支持后顾语法"},
		{`(?<!x)y`, "Go 不支持后顾语法"},
		{`(?>ab)`, "Go 不支持原子组"},
		{`(a)\1`, `Go 不支持反向引用 \1`},
		{`(a)(b)\2`, `Go 不支持反向引用 \2`},
		{`a++`, "Go 不支持占有量词"},
		{`a?+`, "Go 不支持占有量词"},
		{`[`, "missing closing ]"},
		{`\Z`, "invalid escape sequence"},
	}
	for _, c := range cases {
		wantErr(t, c.re, checkRegex(c.re), c.want)
	}
}

func TestCheckRules(t *testing.T) {
	cases := []struct {
		name  string
		rules []string
		want  string
	}{
		{"空列表", nil, ""},
		{"空规则", []string{""}, ""},
		{"全部合法", []string{`720`, `{{ANi}}:繁`}, ""},
		{"报出第一条无效规则", []string{`720`, `(?!x)`, `(?<=y)`}, "全局排除规则「(?!x)」无效：Go 不支持前瞻语法"},
		{"字幕组前缀只校验冒号后部分", []string{`{{ANi}}:(?<=\[)`}, "全局排除规则「{{ANi}}:(?<=\\[)」无效：Go 不支持后顾语法"},
		{"不符合前缀格式时整条校验", []string{`{{ANi}}`}, ""},
	}
	for _, c := range cases {
		wantErr(t, c.name, checkRules("全局排除规则", c.rules), c.want)
	}
}

func TestValidateAniRegex(t *testing.T) {
	cases := []struct {
		name string
		ani  entity.Ani
		want string
	}{
		{"合法", entity.Ani{Match: []string{`1080`}, Exclude: []string{`{{ANi}}:繁`}}, ""},
		{"匹配规则", entity.Ani{Match: []string{`(?<=\[)\d+`}}, "匹配规则「(?<=\\[)\\d+」无效"},
		{"排除规则", entity.Ani{Exclude: []string{`(?!简)`}}, "排除规则「(?!简)」无效"},
		{"自定义集数启用时校验", entity.Ani{CustomEpisode: new(true), CustomEpisodeStr: `(a)\1`}, `自定义集数正则「(a)\1」无效`},
		{"自定义集数未启用不校验", entity.Ani{CustomEpisode: new(false), CustomEpisodeStr: `(a)\1`}, ""},
		{"自定义集数未设置不校验", entity.Ani{CustomEpisodeStr: `(a)\1`}, ""},
	}
	for _, c := range cases {
		wantErr(t, c.name, validateAniRegex(&c.ani), c.want)
	}
}

func TestAniEntryRejectsInvalidRegex(t *testing.T) {
	bad := func(title string) *entity.Ani {
		return &entity.Ani{Title: title, URL: "https://example.com/rss", Exclude: []string{`(?!x)`}}
	}
	good := &entity.Ani{Title: "合法", URL: "https://example.com/rss"}

	wantErr(t, "AddAni", AddAni(bad("A"), false), "排除规则「(?!x)」无效")
	wantErr(t, "SetAni", SetAni(bad("A"), false), "排除规则「(?!x)」无效")
	wantErr(t, "ImportAni", ImportAni([]*entity.Ani{good, bad("B")}, "SKIP"), "订阅「B」的排除规则「(?!x)」无效")
	wantErr(t, "ImportAni nil", ImportAni([]*entity.Ani{good, nil}, "SKIP"), "参数错误")
}

func TestSetConfigRejectsInvalidRegex(t *testing.T) {
	before := slices.Clone(config.Get().Exclude)
	cases := []struct {
		name string
		body string
		want string
	}{
		{"全局排除规则", `{"exclude":["720","(?!x)"]}`, "全局排除规则「(?!x)」无效"},
		{"自定义集数启用时校验", `{"customEpisode":true,"customEpisodeStr":"(a)\\1"}`, `自定义集数正则「(a)\1」无效`},
	}
	for _, c := range cases {
		var body map[string]json.RawMessage
		if err := json.Unmarshal([]byte(c.body), &body); err != nil {
			t.Fatal(err)
		}
		_, err := SetConfig(body)
		wantErr(t, c.name, err, c.want)
	}
	if got := config.Get().Exclude; !slices.Equal(got, before) {
		t.Errorf("校验失败后配置被修改: got %q, want %q", got, before)
	}
}
