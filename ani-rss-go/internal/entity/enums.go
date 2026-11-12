package entity

import (
	"regexp"
)

// 集数提取正则
const EpisodeRegStr = `(.*|\[.*])(( - |Vol |[Ee][Pp]?)\d+(\.5)?( ?\(\d+\))?|【\d+(\.5)?】|\[\d+(\.5)?( ?\(\d+\))?( ?[vV]\d)?( ?END)?( ?完)?( ?FIN)?]|第\d+(\.5)?[话話集]( - END)?|^\[TOC].* \d+|^六四位元字幕组.*★\d+(\.5)?★)`

// 常用正则
var (
	EpisodeReg  = regexp.MustCompile(EpisodeRegStr)
	SeasonReg   = regexp.MustCompile(`[Ss](\d+)[Ee](\d+(\.5)?)`)
	YearReg     = regexp.MustCompile(` ?\(((19|20)\d{2})\)`)
	TmdbIdReg   = regexp.MustCompile(` ?(\[tmdbid=(\d+)]|\{tmdb-(\d+)})`)
	MagnetReg   = regexp.MustCompile(`^magnet\:\?xt=urn:btih\:(\w+)`)
	SubgroupRe  = regexp.MustCompile(`^\{\{(.+)}}:(.+)$`)
	Ed2kReg     = regexp.MustCompile(`^ed2k://\|file\|([^|]+)\|(\d+)\|([A-Fa-f0-9]{32})\|/$`)
	Resolution  = regexp.MustCompile(`(720|1080|2160)[Pp]`)
	SubgroupTag = regexp.MustCompile(`^\[(.+?)]`)
	HashSuffix  = regexp.MustCompile(`\[([A-Z]|\d){8}]$`)
	// WinDriveReg Windows 盘符前缀
	WinDriveReg = regexp.MustCompile(`^[A-Za-z]:`)
)

// SubgroupUnknown 无法识别字幕组时的名称
const SubgroupUnknown = "未知字幕组"

// 种子标签
const (
	TagAniRss           = "ani-rss"
	TagRename           = "RENAME"
	TagStandbyRss       = "备用RSS"
	TagDownloadComplete = "下载完成"
)

// NotificationStatus 通知状态，Name 是通知配置 statusList 中保存的枚举名
type NotificationStatus struct {
	Name   string
	Emoji  string
	Action string
}

var (
	StatusDownloadStart   = NotificationStatus{Name: "DOWNLOAD_START", Emoji: "🎈", Action: "开始下载"}
	StatusDownloadEnd     = NotificationStatus{Name: "DOWNLOAD_END", Emoji: "🎉", Action: "下载完成"}
	StatusOmit            = NotificationStatus{Name: "OMIT", Emoji: "⛔", Action: "缺少集数"}
	StatusError           = NotificationStatus{Name: "ERROR", Emoji: "❌", Action: "发生错误"}
	StatusCompleted       = NotificationStatus{Name: "COMPLETED", Emoji: "🎊", Action: "订阅完结"}
	StatusProcrastinating = NotificationStatus{Name: "PROCRASTINATING", Emoji: "🐟", Action: "摸鱼检测"}
)

// 通知类型
const (
	NotifyTypeEmbyRefresh    = "EMBY_REFRESH"
	NotifyTypeMail           = "MAIL"
	NotifyTypeServerChan     = "SERVER_CHAN"
	NotifyTypeSystem         = "SYSTEM"
	NotifyTypeTelegram       = "TELEGRAM"
	NotifyTypeWebHook        = "WEB_HOOK"
	NotifyTypeShell          = "SHELL"
	NotifyTypeFileMove       = "FILE_MOVE"
	NotifyTypeOpenListUpload = "OPEN_LIST_UPLOAD"
	NotifyTypeBark           = "BARK"
	NotifyTypeAstrBot        = "ASTRBOT"
)

// 自动偏移方式
const (
	// OffsetAuto 正篇从第 1 集开始
	OffsetAuto = "AUTO_OFFSET"
	// OffsetBgm 正篇从 Bangumi 正篇最小集数开始
	OffsetBgm = "BGM_OFFSET"
)

// BGM Token 类型
const (
	BgmTokenInput = "INPUT"
	BgmTokenAuto  = "AUTO"
)

// 排序方式
const (
	SortScore        = "SCORE"
	SortPinyin       = "PINYIN"
	SortDownloadTime = "DOWNLOAD_TIME"
)

// qBittorrent 下载完成状态
var FinishedStates = []string{"queuedUP", "uploading", "stalledUP", "stoppedUP"}
