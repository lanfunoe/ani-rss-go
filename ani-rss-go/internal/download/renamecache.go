// 重命名缓存：Transmission 与 Aria2 记录待重命名任务，持久化为 cache/rename-cache.json，条目一小时后过期
package download

import (
	"cmp"
	"encoding/json"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/utils"
)

// renameCacheEntry 缓存条目，Time 为过期时间的毫秒时间戳，小于 1 表示永不过期
type renameCacheEntry struct {
	Time  int64  `json:"time"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (c renameCacheEntry) expired(now time.Time) bool {
	return c.Time >= 1 && now.UnixMilli() >= c.Time
}

var (
	renameCacheMu     sync.Mutex
	renameCacheLoaded bool
	renameCacheData   map[string]renameCacheEntry
)

func renameCacheFile() string {
	return filepath.Join(config.ConfigDir(), "cache", "rename-cache.json")
}

func loadRenameCache() {
	if renameCacheLoaded {
		return
	}
	renameCacheLoaded = true
	renameCacheData = make(map[string]renameCacheEntry)
	content, err := os.ReadFile(renameCacheFile())
	if err != nil {
		return
	}
	var list []renameCacheEntry
	if err := json.Unmarshal(content, &list); err != nil {
		slog.Error("读取重命名缓存失败", "file", renameCacheFile(), "error", err)
		return
	}
	for _, c := range list {
		renameCacheData[c.Key] = c
	}
}

// saveRenameCache 清理过期条目后写入磁盘
func saveRenameCache() {
	now := time.Now()
	maps.DeleteFunc(renameCacheData, func(_ string, c renameCacheEntry) bool { return c.expired(now) })
	list := slices.SortedFunc(maps.Values(renameCacheData), func(a, b renameCacheEntry) int { return cmp.Compare(a.Time, b.Time) })
	content, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}
	file := renameCacheFile()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		slog.Error("保存重命名缓存失败", "file", file, "error", err)
		return
	}
	if err := utils.WriteFileAtomic(file, content, 0o644); err != nil {
		slog.Error("保存重命名缓存失败", "file", file, "error", err)
	}
}

// putRenameCache 写入重命名缓存，一小时后过期
func putRenameCache(id, name string) {
	renameCacheMu.Lock()
	defer renameCacheMu.Unlock()
	loadRenameCache()
	renameCacheData[id] = renameCacheEntry{Time: time.Now().Add(time.Hour).UnixMilli(), Key: id, Value: name}
	saveRenameCache()
}

// getRenameCache 读取重命名缓存，不存在或已过期返回空串
func getRenameCache(id string) string {
	renameCacheMu.Lock()
	defer renameCacheMu.Unlock()
	loadRenameCache()
	c, ok := renameCacheData[id]
	if !ok || c.expired(time.Now()) {
		return ""
	}
	return c.Value
}

// removeRenameCache 删除重命名缓存
func removeRenameCache(id string) {
	renameCacheMu.Lock()
	defer renameCacheMu.Unlock()
	loadRenameCache()
	delete(renameCacheData, id)
	saveRenameCache()
}
