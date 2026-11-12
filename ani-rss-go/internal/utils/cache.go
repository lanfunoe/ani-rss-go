package utils

import (
	"github.com/jellydator/ttlcache/v3"
)

// Cache 全局缓存，容量上限 8192，读取不延长有效期
var Cache = ttlcache.New(
	ttlcache.WithCapacity[string, any](8192),
	ttlcache.WithDisableTouchOnHit[string, any](),
)

func init() {
	// 定期清理过期条目
	go Cache.Start()
}

// CacheGet 读取未过期的缓存值，不存在、已过期或类型不符时返回 false
func CacheGet[T any](key string) (T, bool) {
	item := Cache.Get(key)
	if item == nil {
		var zero T
		return zero, false
	}
	v, ok := item.Value().(T)
	return v, ok
}
