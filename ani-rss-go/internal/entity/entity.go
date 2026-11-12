// Package entity 数据实体、枚举与全局常量
package entity

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"ani-rss-go/internal/utils"
)

// Date 日期序列化为 yyyy-MM-dd
type Date struct{ time.Time }

func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte(`""`), nil
	}
	return []byte(`"` + d.Format("2006-01-02") + `"`), nil
}

func (d *Date) UnmarshalJSON(b []byte) error {
	s := string(b)
	if len(s) < 2 || s == `""` || s == "null" {
		return nil
	}
	s = s[1 : len(s)-1]
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339, "2006-01", "2006"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			d.Time = t
			return nil
		}
	}
	return nil
}

// Login 登录信息
type Login struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Tmdb TMDB 信息
type Tmdb struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	OriginalName string `json:"originalName"`
	Date         *Date  `json:"date"`
	GroupID      string `json:"tmdbGroupId"`
}

// StandbyRss 备用RSS
type StandbyRss struct {
	Label  string `json:"label"`
	URL    string `json:"url"`
	Offset int    `json:"offset"`
}

// GroupRegexItem 自定义重命名正则项
type GroupRegexItem struct {
	Label string `json:"label"`
	Regex string `json:"regex"`
}

// GroupRegex 自定义重命名规则
type GroupRegex struct {
	RegexList [][]GroupRegexItem `json:"regexList"`
	Tags      []string           `json:"tags"`
}

// Ani 订阅
type Ani struct {
	Sort                         int          `json:"sort"`
	ID                           string       `json:"id"`
	MikanTitle                   string       `json:"mikanTitle"`
	URL                          string       `json:"url"`
	Exists                       *bool        `json:"exists"`
	StandbyRssList               []StandbyRss `json:"standbyRssList"`
	Title                        string       `json:"title"`
	JpTitle                      string       `json:"jpTitle"`
	Offset                       int          `json:"offset"`
	ReleaseDate                  Date         `json:"releaseDate"`
	Year                         int          `json:"year"`
	Month                        int          `json:"month"`
	Day                          int          `json:"date"`
	WeekLabel                    string       `json:"weekLabel"`
	Season                       int          `json:"season"`
	Cover                        string       `json:"cover"`
	Image                        string       `json:"image"`
	Subgroup                     string       `json:"subgroup"`
	Match                        []string     `json:"match"`
	Exclude                      []string     `json:"exclude"`
	GlobalExclude                *bool        `json:"globalExclude"`
	Ova                          *bool        `json:"ova"`
	Pinyin                       string       `json:"pinyin"`
	PinyinInitials               string       `json:"pinyinInitials"`
	Enable                       *bool        `json:"enable"`
	CurrentEpisodeNumber         int          `json:"currentEpisodeNumber"`
	TotalEpisodeNumber           int          `json:"totalEpisodeNumber"`
	ThemoviedbName               string       `json:"themoviedbName"`
	Type                         string       `json:"type"`
	BgmUrl                       string       `json:"bgmUrl"`
	CustomDownloadPath           *bool        `json:"customDownloadPath"`
	CustomDownloadPathTemplate   string       `json:"customDownloadPathTemplate"`
	Score                        float64      `json:"score"`
	CustomEpisode                *bool        `json:"customEpisode"`
	CustomEpisodeStr             string       `json:"customEpisodeStr"`
	CustomEpisodeGroupIndex      int          `json:"customEpisodeGroupIndex"`
	Omit                         *bool        `json:"omit"`
	DownloadNew                  *bool        `json:"downloadNew"`
	NotDownload                  []float64    `json:"notDownload"`
	Tmdb                         *Tmdb        `json:"tmdb"`
	Upload                       *bool        `json:"upload"`
	Procrastinating              *bool        `json:"procrastinating"`
	CustomRenameTemplateEnable   *bool        `json:"customRenameTemplateEnable"`
	CustomRenameTemplate         string       `json:"customRenameTemplate"`
	CustomPriorityKeywordsEnable *bool        `json:"customPriorityKeywordsEnable"`
	CustomPriorityKeywords       []string     `json:"customPriorityKeywords"`
	LastDownloadTime             int64        `json:"lastDownloadTime"`
	CustomUploadEnable           *bool        `json:"customUploadEnable"`
	CustomUploadPathTarget       string       `json:"customUploadPathTarget"`
	Message                      *bool        `json:"message"`
	Completed                    *bool        `json:"completed"`
	CustomCompleted              *bool        `json:"customCompleted"`
	CustomCompletedPathTemplate  string       `json:"customCompletedPathTemplate"`
	CustomTagsEnable             *bool        `json:"customTagsEnable"`
	CustomTags                   []string     `json:"customTags"`
}

func (a *Ani) GetOva() bool             { return utils.Deref(a.Ova) }
func (a *Ani) GetEnable() bool          { return utils.DerefOr(a.Enable, true) }
func (a *Ani) GetOmit() bool            { return utils.DerefOr(a.Omit, true) }
func (a *Ani) GetDownloadNew() bool     { return utils.Deref(a.DownloadNew) }
func (a *Ani) GetMessage() bool         { return utils.DerefOr(a.Message, true) }
func (a *Ani) GetCompletedFlag() bool   { return utils.DerefOr(a.Completed, true) }
func (a *Ani) GetUpload() bool          { return utils.Deref(a.Upload) }
func (a *Ani) GetProcrastinating() bool { return utils.DerefOr(a.Procrastinating, true) }
func (a *Ani) GetGlobalExclude() bool   { return utils.Deref(a.GlobalExclude) }
func (a *Ani) GetCustomDownloadPath() bool {
	return utils.Deref(a.CustomDownloadPath)
}

// Item 资源项
type Item struct {
	Title         string     `json:"title"`
	ReName        string     `json:"reName"`
	Torrent       string     `json:"torrent"`
	InfoHash      string     `json:"infoHash"`
	Episode       float64    `json:"episode"`
	FormatSize    string     `json:"formatSize"`
	Length        int64      `json:"length"`
	HasDownloaded *bool      `json:"hasDownloaded"`
	TorrentPath   string     `json:"torrentPath"`
	Master        *bool      `json:"master"`
	Subgroup      string     `json:"subgroup"`
	PubDate       *time.Time `json:"pubDate"`
}

func (i *Item) GetMaster() bool        { return utils.Deref(i.Master) }
func (i *Item) GetHasDownloaded() bool { return utils.Deref(i.HasDownloaded) }

// EmbyView Emby 媒体库
type EmbyView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// NotificationConfig 通知配置
type NotificationConfig struct {
	Enable               *bool  `json:"enable"`
	Retry                int    `json:"retry"`
	Comment              string `json:"comment"`
	NotificationTemplate string `json:"notificationTemplate"`
	NotificationType     string `json:"notificationType"`

	MailSMTPHost  string `json:"mailSMTPHost"`
	MailSMTPPort  int    `json:"mailSMTPPort"`
	MailFrom      string `json:"mailFrom"`
	MailPassword  string `json:"mailPassword"`
	MailSSLEnable *bool  `json:"mailSSLEnable"`
	MailTLSEnable *bool  `json:"mailTLSEnable"`
	MailAddressee string `json:"mailAddressee"`
	MailImage     *bool  `json:"mailImage"`

	ServerChanType        string `json:"serverChanType"`
	ServerChanSendKey     string `json:"serverChanSendKey"`
	ServerChan3ApiUrl     string `json:"serverChan3ApiUrl"`
	ServerChanTitleAction *bool  `json:"serverChanTitleAction"`

	SystemMsg *bool `json:"systemMsg"`

	TelegramBotToken string `json:"telegramBotToken"`
	TelegramChatId   string `json:"telegramChatId"`
	TelegramTopicId  int    `json:"telegramTopicId"`
	TelegramApiHost  string `json:"telegramApiHost"`
	TelegramImage    *bool  `json:"telegramImage"`
	TelegramFormat   string `json:"telegramFormat"`

	WebHookMethod string `json:"webHookMethod"`
	WebHookUrl    string `json:"webHookUrl"`
	WebHookHeader string `json:"webHookHeader"`
	WebHookBody   string `json:"webHookBody"`

	EmbyRefresh        *bool    `json:"embyRefresh"`
	EmbyHost           string   `json:"embyHost"`
	EmbyApiKey         string   `json:"embyApiKey"`
	EmbyRefreshViewIds []string `json:"embyRefreshViewIds"`
	EmbyDelayed        int64    `json:"embyDelayed"`

	Shell      string `json:"shell"`
	AliveLimit int    `json:"aliveLimit"`

	FileMoveTarget           string `json:"fileMoveTarget"`
	FileMoveOvaTarget        string `json:"fileMoveOvaTarget"`
	FileMoveDeleteOldEpisode *bool  `json:"fileMoveDeleteOldEpisode"`
	FileMoveCopyModel        *bool  `json:"fileMoveCopyModel"`

	OpenListUploadHost             string `json:"openListUploadHost"`
	OpenListUploadApiKey           string `json:"openListUploadApiKey"`
	OpenListUploadPath             string `json:"openListUploadPath"`
	OpenListUploadOvaPath          string `json:"openListUploadOvaPath"`
	OpenListUploadDeleteLocalFile  *bool  `json:"openListUploadDeleteLocalFile"`
	OpenListUploadDeleteOldEpisode *bool  `json:"openListUploadDeleteOldEpisode"`

	BarkServerUrl   string   `json:"barkServerUrl"`
	BarkDeviceKeys  []string `json:"barkDeviceKeys"`
	BarkGroup       string   `json:"barkGroup"`
	BarkUseMarkdown *bool    `json:"barkUseMarkdown"`
	BarkLevel       string   `json:"barkLevel"`
	BarkVolume      int      `json:"barkVolume"`

	// AstrBotApiKey 需要 im 权限，发送封面时还需要 file 权限
	AstrBotHost   string `json:"astrBotHost"`
	AstrBotApiKey string `json:"astrBotApiKey"`
	// AstrBotUmo 目标会话的统一消息来源标识，在目标会话发送 /sid 获取
	AstrBotUmo   string `json:"astrBotUmo"`
	AstrBotImage *bool  `json:"astrBotImage"`

	StatusList []string `json:"statusList"`
	Sort       int64    `json:"sort"`
}

func (n *NotificationConfig) GetEnable() bool { return utils.DerefOr(n.Enable, true) }

// TorrentsInfo 种子信息
type TorrentsInfo struct {
	ID         string   `json:"id"`
	Hash       string   `json:"hash"`
	Name       string   `json:"name"`
	State      string   `json:"state"`
	Category   string   `json:"category"`
	TagList    []string `json:"tagList"`
	Completed  int64    `json:"completed"`
	Size       int64    `json:"size"`
	Progress   float64  `json:"progress"`
	FormatSize string   `json:"formatSize"`
	SavePath   string   `json:"savePath"`
}

// Finished 是否下载完成
func (t *TorrentsInfo) Finished() bool {
	if t.Progress < 100 {
		return false
	}
	return slices.Contains(FinishedStates, t.State)
}

// BgmInfo Bangumi 信息
type BgmInfo struct {
	ID       int              `json:"id"`
	Name     string           `json:"name"`
	NameCn   string           `json:"nameCn"`
	Date     *Date            `json:"date"`
	Summary  string           `json:"summary"`
	Images   *BgmImages       `json:"images"`
	URL      string           `json:"url"`
	Eps      int              `json:"eps"`
	Platform string           `json:"platform"`
	Season   int              `json:"season"`
	Tags     []BgmTag         `json:"tags"`
	Infobox  []BgmInfoboxItem `json:"infobox"`
	Rating   *BgmRating       `json:"rating"`
}

// UnmarshalJSON 兼容 BGM API 的 name_cn 字段
func (b *BgmInfo) UnmarshalJSON(data []byte) error {
	type plain BgmInfo
	aux := struct {
		*plain
		NameCnSnake string `json:"name_cn"`
	}{plain: (*plain)(b)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if b.NameCn == "" {
		b.NameCn = aux.NameCnSnake
	}
	return nil
}

// Image 按配置的尺寸取封面地址，未配置时使用 large
func (b *BgmInfo) Image(size string) string {
	if b.Images == nil {
		return ""
	}
	switch size {
	case "small":
		return b.Images.Small
	case "grid":
		return b.Images.Grid
	case "medium":
		return b.Images.Medium
	case "common":
		return b.Images.Common
	default:
		return b.Images.Large
	}
}

// BgmImages BGM 封面图片
type BgmImages struct {
	Small  string `json:"small"`
	Grid   string `json:"grid"`
	Large  string `json:"large"`
	Medium string `json:"medium"`
	Common string `json:"common"`
}

// BgmTag BGM 标签
type BgmTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// BgmInfoboxItem BGM 条目信息框，value 可能是字符串或 {k, v} 数组
type BgmInfoboxItem struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type BgmRating struct {
	Rank  int     `json:"rank"`
	Total int     `json:"total"`
	Score float64 `json:"score"`
}

// MikanGroup Mikan 字幕组
type MikanGroup struct {
	Label      string      `json:"label"`
	SubgroupId string      `json:"subgroupId"`
	Rss        string      `json:"rss"`
	UpdateDay  string      `json:"updateDay"`
	Items      []MikanItem `json:"items"`
}

// MikanItem Mikan 条目
type MikanItem struct {
	Title      string `json:"title"`
	Magnet     string `json:"magnet"`
	FormatSize string `json:"formatSize"`
	CreatedAt  string `json:"createdAt"`
	Torrent    string `json:"torrent"`
}

// MikanInfo Mikan 页面信息
type MikanInfo struct {
	URL    string       `json:"url"`
	Cover  string       `json:"cover"`
	Title  string       `json:"title"`
	BgmUrl string       `json:"bgmUrl"`
	Groups []MikanGroup `json:"groups"`
}

// WeekAni 周分组
type WeekAni struct {
	WeekLabel string `json:"weekLabel"`
	Items     []*Ani `json:"items"`
}

// ListAni 订阅列表响应
type ListAni struct {
	WeekList        []WeekAni `json:"weekList"`
	ReleaseDateList []string  `json:"releaseDateList"`
	Total           int       `json:"total"`
}

// Result 统一响应体
type Result struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
	T       int64  `json:"t"`
}

// 响应码
const (
	CodeOK       = 200
	CodeError    = 500
	CodeForbid   = 403
	CodeNotFound = 404
)

// SubjectIdFromUrl 从 bgm.tv/subject/123 中提取 123
func SubjectIdFromUrl(bgmURL string) string {
	_, id, ok := strings.CutLast(bgmURL, "/")
	if !ok {
		return ""
	}
	id = strings.TrimSpace(id)
	if _, err := strconv.Atoi(id); err != nil {
		return ""
	}
	return id
}

func OK(data any) *Result {
	return &Result{Code: CodeOK, Message: "成功", Data: data, T: time.Now().UnixMilli()}
}
func Error(msg string) *Result {
	return &Result{Code: CodeError, Message: msg, T: time.Now().UnixMilli()}
}
func Forbid(msg string) *Result {
	return &Result{Code: CodeForbid, Message: msg, T: time.Now().UnixMilli()}
}
func NotFound() *Result {
	return &Result{Code: CodeNotFound, Message: "404 Not Found !", T: time.Now().UnixMilli()}
}

// BgmAvatar BGM 用户头像
type BgmAvatar struct {
	Large  string `json:"large"`
	Medium string `json:"medium"`
	Small  string `json:"small"`
}

// BgmMe BGM 用户信息
type BgmMe struct {
	Avatar      *BgmAvatar `json:"avatar"`
	ID          int        `json:"id"`
	Sign        string     `json:"sign"`
	URL         string     `json:"url"`
	Username    string     `json:"username"`
	Nickname    string     `json:"nickname"`
	UserGroup   int        `json:"userGroup"`
	RegTime     string     `json:"regTime"`
	Email       string     `json:"email"`
	TimeOffset  int        `json:"timeOffset"`
	ExpiresDays int        `json:"expiresDays"`
}

// DateTime 时间序列化为 yyyy-MM-dd HH:mm:ss，解析兼容 RFC3339
type DateTime struct{ time.Time }

func (d DateTime) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + d.Format(time.DateTime) + `"`), nil
}

func (d *DateTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.DateTime, time.DateOnly} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			d.Time = t
			return nil
		}
	}
	return fmt.Errorf("无法解析时间: %s", s)
}

// FlexString 兼容 JSON 字符串、数字与布尔值的字符串字段
type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" {
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = FlexString(v)
		return nil
	}
	*f = FlexString(s)
	return nil
}

// UpdateInfo GitHub 发布的更新信息
type UpdateInfo struct {
	Latest       string    `json:"latest"`
	DownloadUrl  string    `json:"downloadUrl"`
	Update       bool      `json:"update"`
	AutoUpdate   bool      `json:"autoUpdate"`
	Sha256       string    `json:"sha256"`
	Size         int64     `json:"size"`
	FormatSize   string    `json:"formatSize"`
	MarkdownBody string    `json:"markdownBody"`
	Date         *DateTime `json:"date"`
}

// About 关于信息
type About struct {
	Version string `json:"version"`
	UpdateInfo
}

// WebUI 上传的 WebUI 描述文件 webui.json
type WebUI struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Version  string `json:"version"`
	Filename string `json:"filename"`
}
