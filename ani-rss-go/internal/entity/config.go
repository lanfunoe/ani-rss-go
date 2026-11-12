// Package entity — Config 全局设置
package entity

import (
	"ani-rss-go/internal/utils"
)

// Config 全局设置
type Config struct {
	MikanHost                        string               `json:"mikanHost"`
	TmdbApi                          string               `json:"tmdbApi"`
	TmdbApiKey                       string               `json:"tmdbApiKey"`
	TmdbImage                        string               `json:"tmdbImage"`
	TmdbAnime                        *bool                `json:"tmdbAnime"`
	DownloadToolType                 string               `json:"downloadToolType"`
	DownloadRetry                    *int                 `json:"downloadRetry"`
	DownloadToolHost                 string               `json:"downloadToolHost"`
	DownloadToolUsername             string               `json:"downloadToolUsername"`
	DownloadToolPassword             string               `json:"downloadToolPassword"`
	QbUseDownloadPath                *bool                `json:"qbUseDownloadPath"`
	QbContentLayout                  string               `json:"qbContentLayout"`
	RatioLimit                       *int                 `json:"ratioLimit"`
	SeedingTimeLimit                 *int                 `json:"seedingTimeLimit"`
	InactiveSeedingTimeLimit         *int                 `json:"inactiveSeedingTimeLimit"`
	DownloadPathTemplate             string               `json:"downloadPathTemplate"`
	OvaDownloadPathTemplate          string               `json:"ovaDownloadPathTemplate"`
	CompletedPathTemplate            string               `json:"completedPathTemplate"`
	CustomTags                       []string             `json:"customTags"`
	PriorityKeywordsEnable           *bool                `json:"priorityKeywordsEnable"`
	PriorityKeywords                 []string             `json:"priorityKeywords"`
	DelayedDownload                  *int                 `json:"delayedDownload"`
	RssSleepMinutes                  *int                 `json:"rssSleepMinutes"`
	RenameSleepSeconds               *int                 `json:"renameSleepSeconds"`
	Rename                           *bool                `json:"rename"`
	Rss                              *bool                `json:"rss"`
	RssTimeout                       *int                 `json:"rssTimeout"`
	FileExist                        *bool                `json:"fileExist"`
	AwaitStalledUP                   *bool                `json:"awaitStalledUP"`
	Delete                           *bool                `json:"delete"`
	DeleteStandbyRSSOnly             *bool                `json:"deleteStandbyRSSOnly"`
	Offset                           *bool                `json:"offset"`
	OffsetType                       string               `json:"offsetType"`
	TitleYear                        *bool                `json:"titleYear"`
	AutoDisabled                     *bool                `json:"autoDisabled"`
	Skip5                            *bool                `json:"skip5"`
	StandbyRss                       *bool                `json:"standbyRss"`
	Coexist                          *bool                `json:"coexist"`
	LogsMax                          *int                 `json:"logsMax"`
	Debug                            *bool                `json:"debug"`
	ProcrastinatingMasterOnly        *bool                `json:"procrastinatingMasterOnly"`
	Proxy                            *bool                `json:"proxy"`
	ProxyHost                        string               `json:"proxyHost"`
	ProxyPort                        *int                 `json:"proxyPort"`
	ProxyUsername                    string               `json:"proxyUsername"`
	ProxyPassword                    string               `json:"proxyPassword"`
	DownloadCount                    *int                 `json:"downloadCount"`
	Login                            *Login               `json:"login"`
	MultiLoginForbidden              *bool                `json:"multiLoginForbidden"`
	LoginEffectiveHours              *int                 `json:"loginEffectiveHours"`
	Exclude                          []string             `json:"exclude"`
	ImportExclude                    *bool                `json:"importExclude"`
	EnabledExclude                   *bool                `json:"enabledExclude"`
	BgmJpName                        *bool                `json:"bgmJpName"`
	Tmdb                             *bool                `json:"tmdb"`
	TmdbId                           *bool                `json:"tmdbId"`
	TmdbIdPlexMode                   *bool                `json:"tmdbIdPlexMode"`
	TmdbLanguage                     string               `json:"tmdbLanguage"`
	TmdbRomaji                       *bool                `json:"tmdbRomaji"`
	TmdbOriginalName                 *bool                `json:"tmdbOriginalName"`
	IpWhitelist                      *bool                `json:"ipWhitelist"`
	IpWhitelistStr                   string               `json:"ipWhitelistStr"`
	Omit                             *bool                `json:"omit"`
	BgmTokenType                     string               `json:"bgmTokenType"`
	BgmToken                         string               `json:"bgmToken"`
	BgmAppID                         string               `json:"bgmAppID"`
	BgmAppSecret                     string               `json:"bgmAppSecret"`
	BgmRefreshToken                  string               `json:"bgmRefreshToken"`
	BgmRedirectUri                   string               `json:"bgmRedirectUri"`
	ApiKey                           string               `json:"apiKey"`
	DownloadNew                      *bool                `json:"downloadNew"`
	InnerIP                          *bool                `json:"innerIP"`
	RenameTemplate                   string               `json:"renameTemplate"`
	RenameDelYear                    *bool                `json:"renameDelYear"`
	RenameDelTmdbId                  *bool                `json:"renameDelTmdbId"`
	VerifyLoginIp                    *bool                `json:"verifyLoginIp"`
	AutoTrackersUpdate               *bool                `json:"autoTrackersUpdate"`
	TrackersUpdateUrls               string               `json:"trackersUpdateUrls"`
	NotificationTemplate             string               `json:"notificationTemplate"`
	AutoUpdate                       *bool                `json:"autoUpdate"`
	Version                          string               `json:"version"`
	BgmImageSize                     string               `json:"bgmImageSize"`
	CustomCss                        string               `json:"customCss"`
	CustomJs                         string               `json:"customJs"`
	CustomEpisode                    *bool                `json:"customEpisode"`
	CustomEpisodeStr                 string               `json:"customEpisodeStr"`
	CustomEpisodeGroupIndex          *int                 `json:"customEpisodeGroupIndex"`
	Upload                           *bool                `json:"upload"`
	UpLimit                          *int64               `json:"upLimit"`
	DlLimit                          *int64               `json:"dlLimit"`
	ExpirationTime                   *int64               `json:"expirationTime"`
	OutTradeNo                       string               `json:"outTradeNo"`
	VerifyExpirationTime             *bool                `json:"verifyExpirationTime"`
	TryOut                           *bool                `json:"tryOut"`
	Procrastinating                  *bool                `json:"procrastinating"`
	ProcrastinatingDay               *int                 `json:"procrastinatingDay"`
	GithubToken                      string               `json:"githubToken"`
	UpdateTotalEpisodeNumber         *bool                `json:"updateTotalEpisodeNumber"`
	ForceUpdateTotalEpisodeNumber    *bool                `json:"forceUpdateTotalEpisodeNumber"`
	ConfigBackup                     *bool                `json:"configBackup"`
	ConfigBackupDay                  *int                 `json:"configBackupDay"`
	Completed                        *bool                `json:"completed"`
	NotificationConfigList           []NotificationConfig `json:"notificationConfigList"`
	SortType                         string               `json:"sortType"`
	ProxyList                        string               `json:"proxyList"`
	Scrape                           *bool                `json:"scrape"`
	FollowDay                        *int                 `json:"followDay"`
	BangumiIniEnabled                *bool                `json:"bangumiIniEnabled"`
	MaxFileNameLength                *int                 `json:"maxFileNameLength"`
	LimitLoginAttempts               *bool                `json:"limitLoginAttempts"`
	GitInfo                          any                  `json:"gitInfo"`
	ReverseProxyTrustIpListEnabled   *bool                `json:"reverseProxyTrustIpListEnabled"`
	ReverseProxyTrustIpList          []string             `json:"reverseProxyTrustIpList"`
	SubtitleIndependentFolderEnabled *bool                `json:"subtitleIndependentFolderEnabled"`
	SubtitleIndependentFolderName    string               `json:"subtitleIndependentFolderName"`
	BgmApi                           string               `json:"bgmApi"`
	AutoStart                        *bool                `json:"autoStart"`
	AllowCors                        *bool                `json:"allowCors"`
	Uuid                             string               `json:"uuid"`
	JwtKey                           string               `json:"jwtKey"`
	TokenId                          string               `json:"tokenId"`
}

// 布尔读取辅助
func (c *Config) GetRss() bool        { return utils.Deref(c.Rss) }
func (c *Config) GetRename() bool     { return utils.Deref(c.Rename) }
func (c *Config) GetSkip5() bool      { return utils.Deref(c.Skip5) }
func (c *Config) GetOmit() bool       { return utils.DerefOr(c.Omit, true) }
func (c *Config) GetDebug() bool      { return utils.Deref(c.Debug) }
func (c *Config) GetStandbyRss() bool { return utils.Deref(c.StandbyRss) }
func (c *Config) GetCoexist() bool    { return utils.Deref(c.Coexist) }
func (c *Config) GetDelete() bool     { return utils.Deref(c.Delete) }
func (c *Config) GetDeleteStandbyRSSOnly() bool {
	return utils.Deref(c.DeleteStandbyRSSOnly)
}
func (c *Config) GetAutoDisabled() bool    { return utils.Deref(c.AutoDisabled) }
func (c *Config) GetFileExist() bool       { return utils.Deref(c.FileExist) }
func (c *Config) GetOffset() bool          { return utils.Deref(c.Offset) }
func (c *Config) GetProxy() bool           { return utils.Deref(c.Proxy) }
func (c *Config) GetScrape() bool          { return utils.Deref(c.Scrape) }
func (c *Config) GetProcrastinating() bool { return utils.Deref(c.Procrastinating) }
func (c *Config) GetProcrastinatingMasterOnly() bool {
	return utils.DerefOr(c.ProcrastinatingMasterOnly, true)
}
func (c *Config) GetImportExclude() bool   { return utils.Deref(c.ImportExclude) }
func (c *Config) GetEnabledExclude() bool  { return utils.Deref(c.EnabledExclude) }
func (c *Config) GetDownloadNew() bool     { return utils.Deref(c.DownloadNew) }
func (c *Config) GetRenameDelYear() bool   { return utils.Deref(c.RenameDelYear) }
func (c *Config) GetRenameDelTmdbId() bool { return utils.Deref(c.RenameDelTmdbId) }
func (c *Config) GetPriorityKeywordsEnable() bool {
	return utils.Deref(c.PriorityKeywordsEnable)
}
func (c *Config) GetLimitLoginAttempts() bool {
	return utils.DerefOr(c.LimitLoginAttempts, true)
}
func (c *Config) GetMultiLoginForbidden() bool {
	return utils.DerefOr(c.MultiLoginForbidden, true)
}
func (c *Config) GetVerifyLoginIp() bool { return utils.Deref(c.VerifyLoginIp) }
func (c *Config) GetAllowCors() bool     { return utils.Deref(c.AllowCors) }
func (c *Config) GetInnerIP() bool       { return utils.Deref(c.InnerIP) }
func (c *Config) GetIpWhitelist() bool   { return utils.Deref(c.IpWhitelist) }
func (c *Config) GetAutoTrackersUpdate() bool {
	return utils.Deref(c.AutoTrackersUpdate)
}
func (c *Config) GetAutoUpdate() bool        { return utils.Deref(c.AutoUpdate) }
func (c *Config) GetAutoStart() bool         { return utils.Deref(c.AutoStart) }
func (c *Config) GetConfigBackup() bool      { return utils.Deref(c.ConfigBackup) }
func (c *Config) GetCustomEpisodeFlag() bool { return utils.Deref(c.CustomEpisode) }
