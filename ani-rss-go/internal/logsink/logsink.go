// Package logsink 日志内存副本
package logsink

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/utils"
)

// LogEntry 日志条目
type LogEntry struct {
	// Ts 日志时间的毫秒时间戳
	Ts         int64  `json:"ts"`
	Message    string `json:"message"`
	Level      string `json:"level"`
	LoggerName string `json:"loggerName"`
	ThreadName string `json:"threadName"`
}

// store 全局共享的日志列表
var store = &list{}

type list struct {
	mu      sync.Mutex
	entries []LogEntry
}

// Handler 在内部 Handler 输出的同时保存一份日志到内存
type Handler struct {
	inner slog.Handler
	mem   *list
}

// New 创建带内存副本的日志处理器
func New(inner slog.Handler) *Handler {
	return &Handler{inner: inner, mem: store}
}

// Enabled 未开启 debug 设置时只输出 info 及以上级别
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	if level < slog.LevelInfo && !config.Get().GetDebug() {
		return false
	}
	return h.inner.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	// 消息正文附带结构化属性，时间与级别由前端按 ts、level 展示
	var b strings.Builder
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
		return true
	})
	entry := LogEntry{
		Ts:         r.Time.UnixMilli(),
		Message:    b.String(),
		Level:      r.Level.String(),
		LoggerName: loggerName,
		ThreadName: "-",
	}
	limit := maxEntries()
	h.mem.mu.Lock()
	h.mem.entries = append(h.mem.entries, entry)
	if n := len(h.mem.entries); n > limit {
		h.mem.entries = h.mem.entries[n-limit:]
	}
	h.mem.mu.Unlock()
	return h.inner.Handle(ctx, r)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{inner: h.inner.WithAttrs(attrs), mem: h.mem}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{inner: h.inner.WithGroup(name), mem: h.mem}
}

// loggerName 类路径（slog 无记录器名，固定取包名）
const loggerName = "ani-rss"

// maxEntries 最大日志条数（最多 512 条）
func maxEntries() int {
	if n := utils.Deref(config.Get().LogsMax); n > 0 {
		return min(n, 512)
	}
	return 512
}

// Entries 日志副本
func Entries() []LogEntry {
	store.mu.Lock()
	defer store.mu.Unlock()
	// 非 nil 切片，清空后序列化为 []
	return append(make([]LogEntry, 0, len(store.entries)), store.entries...)
}

// Clear 清空日志
func Clear() {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.entries = nil
}
