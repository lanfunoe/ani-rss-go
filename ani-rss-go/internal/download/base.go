// Package download 下载器抽象与实现
package download

import (
	"cmp"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/utils"
)

// Base 下载器接口
type Base interface {
	Login(cfg *entity.Config) bool
	GetTorrentsInfos() []*entity.TorrentsInfo
	Download(ani *entity.Ani, item *entity.Item, savePath string, torrentFile string) bool
	Delete(info *entity.TorrentsInfo, deleteFiles bool) bool
	Rename(info *entity.TorrentsInfo) bool
	AddTags(info *entity.TorrentsInfo, tags string) bool
	UpdateTrackers(trackers []string)
	SetSavePath(info *entity.TorrentsInfo, path string)
}

var (
	mu          sync.Mutex
	current     Base
	currentType string
)

// New 按下载器类型创建下载器，未知类型使用 qBittorrent
func New(toolType string) Base {
	switch toolType {
	case "Transmission":
		return &Transmission{}
	case "Aria2":
		return &Aria2{}
	default:
		return &QBitTorrent{}
	}
}

// Get 获取当前下载器（按 downloadToolType 加载，带缓存）
func Get() Base {
	t := config.Get().DownloadToolType
	mu.Lock()
	defer mu.Unlock()
	if current == nil || currentType != t {
		current, currentType = New(t), t
	}
	return current
}

// Login 登录当前下载器，未设置下载位置时不登录
func Login() bool {
	time.Sleep(time.Second)
	cfg := config.Get()
	if strings.TrimSpace(cfg.DownloadPathTemplate) == "" {
		slog.Warn("下载位置未设置")
		return false
	}
	return Get().Login(cfg)
}

// GetFileReName 计算种子内文件的重命名结果
func GetFileReName(name, reName string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return name
	}
	ext = strings.TrimPrefix(ext, ".")
	if utils.IsVideoFormat(ext) {
		return reName + "." + ext
	}
	if utils.IsSubtitleFormat(ext) {
		base := filepath.Base(name)
		mainName := strings.TrimSuffix(base, filepath.Ext(base))
		s := filepath.Ext(mainName)
		if s != "" {
			reName += "." + strings.TrimPrefix(s, ".")
		}
		return reName + "." + ext
	}
	return name
}

// NewTags 计算新任务的标签
func NewTags(ani *entity.Ani, item *entity.Item) []string {
	tags := []string{entity.TagAniRss, cmp.Or(item.Subgroup, entity.SubgroupUnknown)}
	if !item.GetMaster() {
		tags = append(tags, entity.TagStandbyRss)
	}
	if utils.Deref(ani.CustomTagsEnable) {
		return append(tags, ani.CustomTags...)
	}
	return append(tags, config.Get().CustomTags...)
}

// setProgress 计算进度、大小与格式化大小
func setProgress(info *entity.TorrentsInfo, completed, size int64) {
	if size < 1 {
		size = 1
		info.Progress = 0
	} else {
		info.Progress = math.Round(float64(completed)/float64(size)*10000) / 100
	}
	info.Completed = completed
	info.Size = size
	info.FormatSize = utils.FormatSize(size, true)
}

// FindAni 根据任务反查订阅，由 service 层注入
var FindAni func(info *entity.TorrentsInfo) *entity.Ani
