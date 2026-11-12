package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"ani-rss-go/internal/entity"
)

// Merge 将 raw 中明确存在的字段覆盖到 dst，仅跳过 JSON null。
// 必须传磁盘/请求的原始 JSON map：值类型字段无法区分「缺失」与「零值」，只有原始 map 能保留存在性。
func Merge(dst *entity.Config, raw map[string]json.RawMessage) error {
	candidate := *dst
	for field, fieldValue := range reflect.ValueOf(&candidate).Elem().Fields() {
		if !field.IsExported() {
			continue
		}
		tag, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if tag == "" || tag == "-" {
			continue
		}
		v, ok := raw[tag]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			continue
		}
		value := reflect.New(field.Type)
		if err := json.Unmarshal(v, value.Interface()); err != nil {
			return fmt.Errorf("配置字段 %s 无效: %w", tag, err)
		}
		fieldValue.Set(value.Elem())
	}
	*dst = candidate
	return nil
}
