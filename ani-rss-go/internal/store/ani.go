// Package store 数据存储：订阅列表、种子文件缓存与封面
package store

import (
	"cmp"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
	"uuid"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/utils"
)

var (
	aniMu   sync.RWMutex
	aniList []*entity.Ani
	saveMu  sync.Mutex
)

// AniFileName 订阅文件名
const AniFileName = "ani.v2.json"

// ANIList 获取订阅列表（只读，请勿修改切片）
func ANIList() []*entity.Ani {
	aniMu.RLock()
	defer aniMu.RUnlock()
	return aniList
}

// FindAni 按 id 查找订阅
func FindAni(id string) *entity.Ani {
	aniMu.RLock()
	defer aniMu.RUnlock()
	if i := slices.IndexFunc(aniList, func(ani *entity.Ani) bool { return ani.ID == id }); i >= 0 {
		return aniList[i]
	}
	return nil
}

// AddAni 添加订阅
func AddAni(ani *entity.Ani) {
	aniMu.Lock()
	defer aniMu.Unlock()
	aniList = append(slices.Clip(aniList), ani)
}

// AniTx 订阅修改事务，持有订阅写锁直到 Commit
// 只能整体替换字段，不能修改切片元素或指针指向的值，Commit 前不能有网络、磁盘操作
type AniTx struct {
	*entity.Ani
	index int
}

// BeginAni 加写锁并返回基于订阅副本的事务，订阅不存在时返回 nil
func BeginAni(id string) *AniTx {
	aniMu.Lock()
	i := slices.IndexFunc(aniList, func(ani *entity.Ani) bool { return ani.ID == id })
	if i < 0 {
		aniMu.Unlock()
		return nil
	}
	updated := *aniList[i]
	return &AniTx{Ani: &updated, index: i}
}

// Commit 在新切片中替换订阅并释放写锁，避免影响 ANIList 返回给读者的底层数组
func (tx *AniTx) Commit() {
	defer aniMu.Unlock()
	list := slices.Clone(aniList)
	list[tx.index] = tx.Ani
	aniList = list
}

// RemoveAni 删除订阅
func RemoveAni(anis ...*entity.Ani) {
	aniMu.Lock()
	defer aniMu.Unlock()
	// 生成新切片，避免影响 ANIList 返回给读者的底层数组
	var out []*entity.Ani
	for _, a := range aniList {
		if !slices.ContainsFunc(anis, func(r *entity.Ani) bool { return r.ID == a.ID }) {
			out = append(out, a)
		}
	}
	aniList = out
}

// AniFile 订阅文件路径
func AniFile() string {
	return filepath.Join(config.ConfigDir(), AniFileName)
}

// LoadAniList 加载订阅
func LoadAniList() error {
	data, err := os.ReadFile(AniFile())
	if errors.Is(err, fs.ErrNotExist) {
		aniMu.Lock()
		aniList = []*entity.Ani{}
		aniMu.Unlock()
		SyncAniList()
		return nil
	}
	if err != nil {
		return err
	}
	// 旧版字段名迁移
	var rawList []map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawList); err == nil {
		migrated := false
		for _, m := range rawList {
			for legacy, key := range map[string]string{
				"downloadPath":    "customDownloadPathTemplate",
				"customAlistPath": "customUploadEnable",
				"alistPath":       "customUploadPathTarget",
			} {
				if v, ok := m[legacy]; ok {
					if _, exists := m[key]; !exists {
						m[key] = v
					}
					delete(m, legacy)
					migrated = true
				}
			}
		}
		if migrated {
			data, _ = json.Marshal(rawList)
		}
	}
	var list []*entity.Ani
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	for _, ani := range list {
		if ani.ReleaseDate.IsZero() {
			// 兼容旧版以年、月、日字段保存的首播日期
			ani.ReleaseDate = entity.Date{Time: time.Now()}
			if ani.Year > 0 && ani.Month > 0 && ani.Day > 0 {
				ani.ReleaseDate = entity.Date{Time: time.Date(ani.Year, time.Month(ani.Month), ani.Day, 0, 0, 0, 0, time.Local)}
			}
		}
		// 自动修补缺失的封面
		ani.Cover = SaveCover(ani.Image)
		// 补全默认字段
		FillAni(ani)
	}
	aniMu.Lock()
	aniList = list
	aniMu.Unlock()
	slog.Debug("加载订阅完成", "count", len(list))
	return nil
}

// SyncAniList 保存订阅
func SyncAniList() {
	saveMu.Lock()
	defer saveMu.Unlock()
	data, err := json.MarshalIndent(ANIList(), "", "  ")
	if err != nil {
		slog.Error("序列化订阅失败", "error", err)
		return
	}
	file := AniFile()
	if err := utils.WriteFileAtomic(file, data, 0o644); err != nil {
		slog.Error("保存订阅失败", "file", file, "error", err)
		return
	}
	slog.Debug("保存订阅成功", "file", file)
}

// CreateAni 新建订阅默认值
func CreateAni() *entity.Ani {
	cfg := config.Get()
	return &entity.Ani{
		ID:                           uuid.New().String(),
		StandbyRssList:               []entity.StandbyRss{},
		ReleaseDate:                  entity.Date{Time: time.Now()},
		Enable:                       new(true),
		Ova:                          new(false),
		CustomDownloadPath:           new(false),
		GlobalExclude:                new(false),
		Match:                        []string{},
		Exclude:                      []string{"720[Pp]", `\d-\d`, "合集", "特别篇"},
		CustomEpisode:                new(cfg.GetCustomEpisodeFlag()),
		CustomEpisodeStr:             cfg.CustomEpisodeStr,
		CustomEpisodeGroupIndex:      utils.Deref(cfg.CustomEpisodeGroupIndex),
		Omit:                         new(true),
		DownloadNew:                  new(false),
		NotDownload:                  []float64{},
		Tmdb:                         &entity.Tmdb{},
		Upload:                       new(utils.Deref(cfg.Upload)),
		Procrastinating:              new(true),
		CustomRenameTemplate:         cfg.RenameTemplate,
		CustomRenameTemplateEnable:   new(false),
		CustomPriorityKeywordsEnable: new(false),
		CustomPriorityKeywords:       []string{},
		Message:                      new(true),
		CustomUploadEnable:           new(false),
		Completed:                    new(true),
		CustomCompleted:              new(false),
		CustomTags:                   []string{},
		CustomTagsEnable:             new(false),
	}
}

// FillAni 用新订阅的默认值补全缺失的字段，保留已有的值
func FillAni(ani *entity.Ani) {
	def := CreateAni()
	ani.Enable = cmp.Or(ani.Enable, def.Enable)
	ani.Ova = cmp.Or(ani.Ova, def.Ova)
	ani.CustomDownloadPath = cmp.Or(ani.CustomDownloadPath, def.CustomDownloadPath)
	ani.GlobalExclude = cmp.Or(ani.GlobalExclude, def.GlobalExclude)
	ani.CustomEpisode = cmp.Or(ani.CustomEpisode, def.CustomEpisode)
	ani.Omit = cmp.Or(ani.Omit, def.Omit)
	ani.DownloadNew = cmp.Or(ani.DownloadNew, def.DownloadNew)
	ani.Upload = cmp.Or(ani.Upload, def.Upload)
	ani.Procrastinating = cmp.Or(ani.Procrastinating, def.Procrastinating)
	ani.CustomRenameTemplateEnable = cmp.Or(ani.CustomRenameTemplateEnable, def.CustomRenameTemplateEnable)
	ani.CustomPriorityKeywordsEnable = cmp.Or(ani.CustomPriorityKeywordsEnable, def.CustomPriorityKeywordsEnable)
	ani.Message = cmp.Or(ani.Message, def.Message)
	ani.CustomUploadEnable = cmp.Or(ani.CustomUploadEnable, def.CustomUploadEnable)
	ani.Completed = cmp.Or(ani.Completed, def.Completed)
	ani.CustomCompleted = cmp.Or(ani.CustomCompleted, def.CustomCompleted)
	ani.CustomTagsEnable = cmp.Or(ani.CustomTagsEnable, def.CustomTagsEnable)
	ani.Tmdb = cmp.Or(ani.Tmdb, def.Tmdb)
	if ani.StandbyRssList == nil {
		ani.StandbyRssList = def.StandbyRssList
	}
	if ani.Match == nil {
		ani.Match = def.Match
	}
	if ani.Exclude == nil {
		ani.Exclude = def.Exclude
	}
	if ani.NotDownload == nil {
		ani.NotDownload = def.NotDownload
	}
	if ani.CustomPriorityKeywords == nil {
		ani.CustomPriorityKeywords = def.CustomPriorityKeywords
	}
	if ani.CustomTags == nil {
		ani.CustomTags = def.CustomTags
	}
	if ani.CustomEpisodeStr == "" {
		ani.CustomEpisodeStr = def.CustomEpisodeStr
	}
	if ani.CustomRenameTemplate == "" {
		ani.CustomRenameTemplate = def.CustomRenameTemplate
	}
}
