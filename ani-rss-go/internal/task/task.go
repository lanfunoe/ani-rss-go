// Package task 后台任务
package task

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/download"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/service"
	"ani-rss-go/internal/store"
	"ani-rss-go/internal/utils"
)

var (
	// mu 保证循环任务的启动与重启串行执行
	mu sync.Mutex
	// cancel 停止当前的循环任务
	cancel context.CancelFunc
	// running 正在运行的循环任务
	running sync.WaitGroup
	// scheduler 每日定时任务调度器
	scheduler    *cron.Cron
	downloadLock atomic.Bool
	lockMu       sync.Mutex
)

// SyncLock 检查并锁定下载（存在未完成任务时报错）
func SyncLock(set bool) error {
	lockMu.Lock()
	defer lockMu.Unlock()
	if downloadLock.Load() {
		return errors.New("存在未完成任务，请等待...")
	}
	if set {
		downloadLock.Store(true)
	}
	return nil
}

// Unlock 解除下载锁
func Unlock() {
	downloadLock.Store(false)
}

// Download 逐个下载已启用且仍存在的订阅，ctx 取消时停止
func Download(ctx context.Context, aniList []*entity.Ani) {
	if !download.Login() {
		return
	}
	for _, ani := range aniList {
		if ctx.Err() != nil {
			return
		}
		if !store.ContainsAni(ani) {
			// 订阅可能已经被删除
			continue
		}
		if !ani.GetEnable() {
			slog.Debug("未启用", "title", ani.Title)
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("下载任务异常", "title", ani.Title, "error", r)
				}
			}()
			service.DownloadAni(ani)
		}()
		// 避免短时间频繁请求导致流控
		sleep(ctx, 500*time.Millisecond)
	}
}

// Start 启动循环任务与每日定时任务
func Start() {
	mu.Lock()
	defer mu.Unlock()
	startLoops()
	cronTask()
}

// Restart 停止循环任务并等待正在执行的任务结束，然后重新启动，修改后的时间间隔立即生效
func Restart() {
	mu.Lock()
	defer mu.Unlock()
	cancel()
	running.Wait()
	startLoops()
}

// startLoops 启动重命名、RSS 与 BGM 循环任务，调用方需持有 mu
func startLoops() {
	var ctx context.Context
	ctx, cancel = context.WithCancel(context.Background())
	for _, t := range []struct {
		name string
		run  func(context.Context)
	}{
		{"rename-task", renameTask},
		{"rss-task", rssTask},
		{"bgm-task", bgmTask},
	} {
		running.Go(func() {
			slog.Info(t.name + " 任务正在运行")
			t.run(ctx)
			slog.Info(t.name + " 任务已停止")
		})
	}
}

// sleep 休眠 d，任务停止时提前返回
func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// rssTask RSS 轮询
func rssTask(ctx context.Context) {
	for ctx.Err() == nil {
		cfg := config.Get()
		if cfg.GetRss() {
			if err := SyncLock(true); err != nil {
				slog.Warn(err.Error())
			} else {
				Download(ctx, store.ANIList())
				Unlock()
			}
		} else {
			slog.Debug("rss未启用")
		}
		sleep(ctx, time.Duration(utils.DerefOr(cfg.RssSleepMinutes, 15))*time.Minute)
	}
}

// renameTask 重命名、下载完成通知与删除已完成任务
func renameTask(ctx context.Context) {
	for ctx.Err() == nil {
		cfg := config.Get()
		sleepDuration := time.Duration(utils.DerefOr(cfg.RenameSleepSeconds, 10)) * time.Second
		if !download.Login() {
			sleep(ctx, sleepDuration)
			continue
		}
		for _, info := range download.GetTorrentsInfos() {
			if ctx.Err() != nil {
				return
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("重命名任务异常", "error", r)
					}
				}()
				download.RenameTorrent(info)
				service.Notification(info)
				if cfg.GetDeleteStandbyRSSOnly() {
					return
				}
				// 删除已完成任务（删除条件由 DeleteTorrent 内部校验）
				download.DeleteTorrent(info, false, false)
			}()
		}
		sleep(ctx, sleepDuration)
	}
}

// bgmTask 刷新 BGM Token，更新评分与总集数
func bgmTask(ctx context.Context) {
	for ctx.Err() == nil {
		service.RefreshToken()
		for _, ani := range store.ANIList() {
			if ctx.Err() != nil {
				return
			}
			if !ani.GetEnable() {
				continue
			}
			info, err := service.GetBgmInfo(ani, false)
			if err != nil {
				continue
			}
			ani.Score = 0
			if info.Rating != nil {
				ani.Score = info.Rating.Score
			}
			cfg := config.Get()
			if !utils.Deref(cfg.UpdateTotalEpisodeNumber) {
				continue
			}
			service.UpdateTotalEpisodeNumber(ani, info, utils.Deref(cfg.ForceUpdateTotalEpisodeNumber))
		}
		store.SyncAniList()
		sleep(ctx, 12*time.Hour)
	}
}

// cronTask 每日定时任务：配置备份、Trackers 更新
func cronTask() {
	scheduler = cron.New(cron.WithSeconds(), cron.WithChain(cron.Recover(cron.DefaultLogger)))
	jobs := []struct {
		spec string
		fn   func()
	}{
		// 每天 0 点备份
		{"0 0 0 * * *", service.Backup},
		// 每天 1 点更新 Trackers
		{"0 0 1 * * *", autoTrackersUpdate},
	}
	for _, job := range jobs {
		if _, err := scheduler.AddFunc(job.spec, job.fn); err != nil {
			slog.Error("添加定时任务失败", "spec", job.spec, "error", err)
		}
	}
	scheduler.Start()
}

// autoTrackersUpdate 开启自动更新 Trackers 时更新下载器 Trackers
func autoTrackersUpdate() {
	cfg := config.Get()
	if !cfg.GetAutoTrackersUpdate() {
		return
	}
	slog.Info("定时任务 开始更新 Trackers")
	if err := download.UpdateTrackers(cfg); err != nil {
		slog.Error("更新 Trackers 失败", "error", err)
	}
}
