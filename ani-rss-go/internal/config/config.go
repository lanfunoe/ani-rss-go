// Package config 全局配置，持久化为 config.v2.json
package config

import (
	"cmp"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"uuid"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/utils"
)

// FileName 配置文件名
const FileName = "config.v2.json"

var (
	mu     sync.RWMutex
	global = Default()
)

// Get 获取全局配置（单例）
func Get() *entity.Config {
	mu.RLock()
	defer mu.RUnlock()
	return global
}

// Default 默认配置
func Default() *entity.Config {
	rootPath := "/Media"
	downloadPath := filepath.Join(rootPath, "番剧")
	ovaPath := filepath.Join(rootPath, "剧场版")
	completedPath := filepath.Join(rootPath, "已完结番剧")

	password := fmt.Sprintf("%x", md5.Sum([]byte("admin")))

	notificationTemplate := `${emoji}${emoji}${emoji}
事件类型: ${action}
标题: ${title}
评分: ${score}
TMDB: ${tmdburl}
TMDB标题: ${themoviedbName}
BGM: ${bgmUrl}
季: ${season}
集: ${episode}
字幕组: ${subgroup}
进度: ${currentEpisodeNumber}/${totalEpisodeNumber}
首播:  ${year}年${month}月${date}日
事件: ${text}
下载位置: ${downloadPath}
TMDB集标题: ${episodeTitle}
${emoji}${emoji}${emoji}`

	proxyList := `mikanani.me
mikanime.tv
anibt.net
animes.garden
nyaa.si
acg.rip
google.com
tmdb.org
themoviedb.org
anilist.co
wushuo.top
bgm.tv
bangumi.tv
chii.in
github.com
raw.githubusercontent.com
telegram.org
`

	c := &entity.Config{
		MikanHost:                 cmp.Or(os.Getenv("MIKAN_HOST"), "https://mikanani.me"),
		TmdbApi:                   "https://api.themoviedb.org",
		TmdbApiKey:                "",
		TmdbImage:                 "https://image.tmdb.org",
		TmdbAnime:                 new(true),
		DownloadToolType:          cmp.Or(os.Getenv("DOWNLOAD_TOOL_TYPE"), "qBittorrent"),
		DownloadRetry:             new(3),
		DownloadToolHost:          os.Getenv("DOWNLOAD_TOOL_HOST"),
		DownloadToolUsername:      os.Getenv("DOWNLOAD_TOOL_USERNAME"),
		DownloadToolPassword:      os.Getenv("DOWNLOAD_TOOL_PASSWORD"),
		QbUseDownloadPath:         new(false),
		QbContentLayout:           "Original",
		RatioLimit:                new(-2),
		SeedingTimeLimit:          new(-2),
		InactiveSeedingTimeLimit:  new(-2),
		DownloadPathTemplate:      filepath.Join(downloadPath, "${title}", "Season ${season}"),
		OvaDownloadPathTemplate:   filepath.Join(ovaPath, "${title}"),
		CompletedPathTemplate:     filepath.Join(completedPath, "${title}", "Season ${season}"),
		CustomTags:                []string{},
		PriorityKeywordsEnable:    new(false),
		PriorityKeywords:          []string{},
		DelayedDownload:           new(0),
		RssSleepMinutes:           new(15),
		RenameSleepSeconds:        new(10),
		Rename:                    new(true),
		Rss:                       new(true),
		RssTimeout:                new(20),
		FileExist:                 new(false),
		AwaitStalledUP:            new(true),
		Delete:                    new(false),
		DeleteStandbyRSSOnly:      new(false),
		Offset:                    new(false),
		OffsetType:                entity.OffsetAuto,
		TitleYear:                 new(true),
		AutoDisabled:              new(false),
		Skip5:                     new(true),
		StandbyRss:                new(false),
		Coexist:                   new(false),
		LogsMax:                   new(128),
		Debug:                     new(false),
		ProcrastinatingMasterOnly: new(true),
		Proxy:                     new(false),
		ProxyHost:                 "",
		ProxyPort:                 new(8080),
		ProxyUsername:             "",
		ProxyPassword:             "",
		DownloadCount:             new(0),
		Login: &entity.Login{
			Username: "admin",
			Password: password,
		},
		MultiLoginForbidden:              new(true),
		LoginEffectiveHours:              new(3),
		Exclude:                          []string{"720[Pp]", `\d-\d`, "合集", "特别篇"},
		ImportExclude:                    new(false),
		EnabledExclude:                   new(false),
		BgmJpName:                        new(false),
		Tmdb:                             new(true),
		TmdbId:                           new(false),
		TmdbIdPlexMode:                   new(false),
		TmdbLanguage:                     "zh-CN",
		TmdbRomaji:                       new(false),
		TmdbOriginalName:                 new(false),
		IpWhitelist:                      new(false),
		IpWhitelistStr:                   "",
		Omit:                             new(true),
		BgmTokenType:                     entity.BgmTokenInput,
		BgmToken:                         "",
		BgmAppID:                         "",
		BgmAppSecret:                     "",
		BgmRefreshToken:                  "",
		BgmRedirectUri:                   "",
		DownloadNew:                      new(false),
		InnerIP:                          new(false),
		RenameTemplate:                   "[${subgroup}] ${title} S${seasonFormat}E${episodeFormat}",
		RenameDelYear:                    new(true),
		RenameDelTmdbId:                  new(true),
		VerifyLoginIp:                    new(false),
		AutoTrackersUpdate:               new(false),
		TrackersUpdateUrls:               "https://cf.trackerslist.com/best.txt",
		NotificationTemplate:             notificationTemplate,
		AutoUpdate:                       new(false),
		Version:                          "",
		BgmImageSize:                     "medium",
		CustomCss:                        "",
		CustomJs:                         "",
		CustomEpisode:                    new(false),
		CustomEpisodeStr:                 entity.EpisodeRegStr,
		CustomEpisodeGroupIndex:          new(2),
		Upload:                           new(true),
		UpLimit:                          new(int64(0)),
		DlLimit:                          new(int64(0)),
		ExpirationTime:                   new(int64(0)),
		OutTradeNo:                       "",
		TryOut:                           new(false),
		VerifyExpirationTime:             new(false),
		Procrastinating:                  new(false),
		ProcrastinatingDay:               new(14),
		GithubToken:                      "",
		UpdateTotalEpisodeNumber:         new(false),
		ForceUpdateTotalEpisodeNumber:    new(false),
		ConfigBackup:                     new(false),
		ConfigBackupDay:                  new(7),
		Completed:                        new(false),
		NotificationConfigList:           []entity.NotificationConfig{},
		SortType:                         entity.SortScore,
		ProxyList:                        proxyList,
		Scrape:                           new(false),
		FollowDay:                        new(14),
		BangumiIniEnabled:                new(false),
		MaxFileNameLength:                new(0),
		LimitLoginAttempts:               new(true),
		ReverseProxyTrustIpList:          []string{"127.0.0.1"},
		ReverseProxyTrustIpListEnabled:   new(false),
		SubtitleIndependentFolderEnabled: new(false),
		SubtitleIndependentFolderName:    "Subs",
		BgmApi:                           "https://api.bgm.tv",
		AutoStart:                        new(false),
		AllowCors:                        new(false),
	}
	c.ApiKey = strings.ToLower(randomString(64))
	c.Uuid = uuid.New().String()
	c.JwtKey = randomKey(32)
	c.TokenId = uuid.New().String()
	return c
}

// ConfigDir 配置目录：CONFIG 环境变量 > ./config，进程内只解析一次
var ConfigDir = sync.OnceValue(func() string {
	if dir := os.Getenv("CONFIG"); dir != "" {
		return dir
	}
	dir, err := filepath.Abs("config")
	if err != nil {
		dir = "config"
	}
	return dir
})

// ConfigFile 配置文件路径
func ConfigFile() string {
	return filepath.Join(ConfigDir(), FileName)
}

// Load 加载配置（文件不存在则写出默认配置）
func Load() error {
	file := ConfigFile()
	if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		data = []byte("{}")
	} else if err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	mu.Lock()
	err = Merge(global, raw) // 仅覆盖磁盘上明确存在的字段
	mu.Unlock()
	if err != nil {
		return err
	}
	// 配置文件不存在或缺少登录令牌的签名密钥时写回磁盘，避免重启后已登录的令牌失效
	_, hasJwtKey := raw["jwtKey"]
	_, hasTokenId := raw["tokenId"]
	if !hasJwtKey || !hasTokenId {
		Sync()
	}
	Format(Get())
	syncHTTPProxy()
	return nil
}

// Sync 保存配置到磁盘（临时文件 + 原子替换）
func Sync() {
	mu.Lock()
	Format(global)
	data, err := json.MarshalIndent(global, "", "  ")
	mu.Unlock()
	if err != nil {
		slog.Error("序列化配置失败", "error", err)
		return
	}
	file := ConfigFile()
	if err := utils.WriteFileAtomic(file, data, 0o644); err != nil {
		slog.Error("保存配置失败", "file", file, "error", err)
		return
	}
	slog.Debug("保存配置成功", "file", file)
	syncHTTPProxy()
}

// syncHTTPProxy 将配置同步到 HTTP 客户端
func syncHTTPProxy() {
	c := Get()
	httpc.SetProxyConfig(httpc.ProxyConfig{
		Enable:    c.GetProxy(),
		Host:      c.ProxyHost,
		Port:      utils.Deref(c.ProxyPort),
		Username:  c.ProxyUsername,
		Password:  c.ProxyPassword,
		ProxyList: c.ProxyList,
	})
}

// Format 格式化 URL 与路径
func Format(c *entity.Config) {
	c.DownloadToolHost = normalizeURL(c.DownloadToolHost)
	c.MikanHost = normalizeURL(c.MikanHost)
	c.TmdbApi = normalizeURL(c.TmdbApi)

	c.DownloadPathTemplate = utils.AbsPath(c.DownloadPathTemplate)
	c.OvaDownloadPathTemplate = utils.AbsPath(c.OvaDownloadPathTemplate)
	c.CompletedPathTemplate = utils.AbsPath(c.CompletedPathTemplate)

	c.NotificationTemplate = strings.TrimSpace(c.NotificationTemplate)
	c.CustomEpisodeStr = cmp.Or(c.CustomEpisodeStr, entity.EpisodeRegStr)
	c.CustomEpisodeGroupIndex = cmp.Or(c.CustomEpisodeGroupIndex, new(2))
	// 补全通知配置默认值（仅补空，不覆盖）
	for i := range c.NotificationConfigList {
		completeNotification(&c.NotificationConfigList[i])
	}
}

// normalizeURL 去除结尾斜杠
func normalizeURL(u string) string {
	return strings.TrimRight(strings.TrimSpace(u), "/")
}

const randomCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomString 指定长度的随机字符串
func randomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(randomCharset))))
		if err != nil {
			panic(err)
		}
		b[i] = randomCharset[n.Int64()]
	}
	return string(b)
}

// randomKey n 字节随机密钥的 Base64 编码
func randomKey(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}
