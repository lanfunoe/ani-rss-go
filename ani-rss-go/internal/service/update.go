// Package service — 版本检测
package service

import (
	"log/slog"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/utils"
)

var aboutMu sync.Mutex

// 发布 Go 版本的 GitHub 仓库
const (
	githubOwner = "lanfunoe"
	githubRepo  = "ani-rss"
)

// GetAbout 检测 GitHub 最新发布版本，结果缓存一分钟
// 下载信息取与当前平台对应的发布产物 ani-rss-go-<GOOS>-<GOARCH>[v<GOARM>]
func GetAbout(version string) *entity.About {
	aboutMu.Lock()
	defer aboutMu.Unlock()
	const key = "github#releases-latest"
	if v, ok := utils.CacheGet[*entity.About](key); ok {
		return v
	}
	about := &entity.About{Version: version}
	defer utils.Cache.Set(key, about, time.Minute)

	filename := "ani-rss-go-" + runtime.GOOS + "-" + runtime.GOARCH
	if info, ok := debug.ReadBuildInfo(); ok && runtime.GOARCH == "arm" {
		for _, s := range info.Settings {
			if s.Key == "GOARM" {
				filename += "v" + s.Value
			}
		}
	}
	info, err := githubUpdateInfo(githubOwner, githubRepo, filename, version)
	if err != nil {
		slog.Error("检测更新失败 " + err.Error())
		return about
	}
	about.UpdateInfo = *info
	return about
}
