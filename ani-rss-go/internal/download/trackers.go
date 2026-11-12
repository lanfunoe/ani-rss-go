// 下载器 Trackers 更新
package download

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
)

// trackerSchemes 有效的 tracker 地址前缀
var trackerSchemes = []string{"udp://", "wss://", "ws://", "https://", "http://"}

// UpdateTrackers 从配置的地址获取 Trackers，登录配置中的下载器后更新
func UpdateTrackers(cfg *entity.Config) error {
	urls := cfg.TrackersUpdateUrls
	if strings.TrimSpace(urls) == "" {
		return errors.New("Trackers更新地址 为空")
	}
	var trackers []string
	for u := range strings.SplitSeq(urls, "\n") {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		slog.Info("获取 tracker", "url", u)
		resp, err := httpc.Get(u).Do()
		if err != nil {
			return fmt.Errorf("更新trackers失败 %s %w", u, err)
		}
		// 仅接受 2xx 的纯文本响应
		contentType := resp.Header.Get("Content-Type")
		if resp.AssertStatus() != nil || !strings.Contains(contentType, "text/plain") {
			resp.Body.Close()
			return fmt.Errorf("更新trackers失败 status: %d contentType: %q %s", resp.StatusCode, contentType, u)
		}
		data, err := resp.BodyBytes()
		if err != nil {
			return err
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			line = strings.Trim(strings.ReplaceAll(line, `"`, ""), " \r")
			if slices.ContainsFunc(trackerSchemes, func(p string) bool { return strings.HasPrefix(line, p) }) &&
				!slices.Contains(trackers, line) {
				trackers = append(trackers, line)
			}
		}
	}
	if len(trackers) == 0 {
		return errors.New("获取到0个trackers, 不进行更新")
	}
	downloader := New(cfg.DownloadToolType)
	if !downloader.Login(cfg) {
		return fmt.Errorf("%s 登录失败", cfg.DownloadToolType)
	}
	downloader.UpdateTrackers(trackers)
	return nil
}
