package config

import (
	"encoding/json"
	"reflect"
	"testing"

	"ani-rss-go/internal/entity"
)

func TestMergePresence(t *testing.T) {
	cfg := &entity.Config{
		RenameTemplate: "template",
		SortType:       "SCORE",
		Rss:            new(true),
		DownloadCount:  new(5),
		Exclude:        []string{"720[Pp]"},
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(`{"renameTemplate":"","rss":false,"downloadCount":0,"exclude":[],"sortType":null}`), &raw); err != nil {
		t.Fatal(err)
	}
	if err := Merge(cfg, raw); err != nil {
		t.Fatal(err)
	}
	if cfg.RenameTemplate != "" || *cfg.Rss || *cfg.DownloadCount != 0 || cfg.Exclude == nil || len(cfg.Exclude) != 0 {
		t.Fatalf("显式零值未保留: %+v", cfg)
	}
	if cfg.SortType != "SCORE" {
		t.Fatalf("null 覆盖了默认值: %q", cfg.SortType)
	}
	if err := Merge(cfg, map[string]json.RawMessage{"githubToken": json.RawMessage(`"token"`)}); err != nil {
		t.Fatal(err)
	}
	if cfg.SortType != "SCORE" || cfg.GithubToken != "token" {
		t.Fatalf("缺失字段或显式值合并错误: %+v", cfg)
	}
}

func TestMergeRejectsInvalidFieldsAtomically(t *testing.T) {
	cfg := &entity.Config{RenameTemplate: "original", DownloadCount: new(3)}
	before, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Merge(cfg, map[string]json.RawMessage{
		"renameTemplate": json.RawMessage(`"changed"`),
		"downloadCount":  json.RawMessage(`"invalid"`),
	}); err == nil {
		t.Fatal("无效字段未返回错误")
	}
	after, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("无效配置请求修改了有效字段")
	}
}
