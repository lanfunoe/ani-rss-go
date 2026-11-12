package entity

import (
	"encoding/json"
	"testing"
)

// TestBgmInfoNameCn 兼容 BGM API 的 name_cn 与前端的 nameCn
func TestBgmInfoNameCn(t *testing.T) {
	var info BgmInfo
	if err := json.Unmarshal([]byte(`{"id":1,"name_cn":"中文","images":{"large":"l","small":"s"}}`), &info); err != nil {
		t.Fatal(err)
	}
	if info.NameCn != "中文" || info.Image("") != "l" || info.Image("small") != "s" {
		t.Fatalf("解析结果错误: %+v", info)
	}
	data, _ := json.Marshal(info)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	if out["nameCn"] != "中文" {
		t.Fatalf("序列化应输出 nameCn: %s", data)
	}
}

// TestFlexString 数字、字符串、布尔值均转为字符串
func TestFlexString(t *testing.T) {
	var v struct {
		A FlexString `json:"a"`
		B FlexString `json:"b"`
		C FlexString `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"a":123,"b":"x","c":false}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.A != "123" || v.B != "x" || v.C != "false" {
		t.Fatalf("解析结果错误: %+v", v)
	}
}

// TestDateTime 解析 RFC3339，输出 yyyy-MM-dd HH:mm:ss
func TestDateTime(t *testing.T) {
	var d DateTime
	if err := json.Unmarshal([]byte(`"2024-01-02T03:04:05Z"`), &d); err != nil {
		t.Fatal(err)
	}
	if d.UTC().Format("2006-01-02 15:04:05") != "2024-01-02 03:04:05" {
		t.Fatalf("解析结果错误: %v", d)
	}
	if err := json.Unmarshal([]byte(`"bad"`), &d); err == nil {
		t.Fatal("无效时间应返回错误")
	}
}
