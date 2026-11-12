// Package service — GitHub 发布检测
package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/utils"
)

// majorMinorReg 版本号前两位，仅允许不跨小版本的自动更新
var majorMinorReg = regexp.MustCompile(`^[Vv]?(\d+\.\d+)`)

// githubUpdateInfo 检测 owner/repo 的最新发布，发布中存在 filename 产物且版本较新时 Update 为 true
func githubUpdateInfo(owner, repo, filename, currentVersion string) (*entity.UpdateInfo, error) {
	info := &entity.UpdateInfo{FormatSize: "0 MiB"}

	req := httpc.Get(fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)).SetTimeout(3 * time.Second)
	if token := config.Get().GithubToken; token != "" {
		req.Set("Authorization", "Bearer "+token)
	}
	resp, err := req.Do()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return info, nil
	}
	if err := resp.AssertStatus(); err != nil {
		return nil, err
	}
	release := struct {
		Message     string          `json:"message"`
		TagName     string          `json:"tag_name"`
		Body        string          `json:"body"`
		PublishedAt entity.DateTime `json:"published_at"`
		Assets      []struct {
			Name               string `json:"name"`
			Size               int64  `json:"size"`
			Digest             string `json:"digest"`
			BrowserDownloadUrl string `json:"browser_download_url"`
		} `json:"assets"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	if release.Message != "" {
		slog.Error(release.Message)
		return info, nil
	}

	latest := strings.ReplaceAll(release.TagName, "v", "")
	latestMajorMinor := majorMinorReg.FindStringSubmatch(latest)
	currentMajorMinor := majorMinorReg.FindStringSubmatch(currentVersion)
	info.Latest = latest
	info.MarkdownBody = release.Body
	info.Date = &release.PublishedAt
	info.AutoUpdate = latestMajorMinor != nil && currentMajorMinor != nil && latestMajorMinor[1] == currentMajorMinor[1]

	update := semver.Compare("v"+latest, "v"+strings.TrimSuffix(currentVersion, "-go")) > 0
	for _, asset := range release.Assets {
		if asset.Name != filename {
			continue
		}
		info.Update = update
		info.DownloadUrl = asset.BrowserDownloadUrl
		info.Sha256 = strings.TrimPrefix(asset.Digest, "sha256:")
		info.Size = asset.Size
		info.FormatSize = utils.FormatSize(asset.Size, true)
	}
	return info, nil
}
