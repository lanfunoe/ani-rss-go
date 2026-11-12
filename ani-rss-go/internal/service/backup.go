package service

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/store"
	"ani-rss-go/internal/utils"
)

// backupMu 定时备份与导出设置互斥
var backupMu sync.Mutex

// Backup 开启自动备份时清理过期备份，并将设置备份为 backup/yyyy-MM-dd.zip，当天已备份则跳过
func Backup() {
	cfg := config.Get()
	if !cfg.GetConfigBackup() {
		return
	}
	backupMu.Lock()
	defer backupMu.Unlock()

	backupDir := filepath.Join(config.ConfigDir(), "backup")
	clearBackup(backupDir, utils.DerefOr(cfg.ConfigBackupDay, 7))

	name := time.Now().Format(time.DateOnly) + ".zip"
	backupFile := filepath.Join(backupDir, name)
	if _, err := os.Stat(backupFile); err == nil {
		return
	}

	slog.Info("正在备份设置 " + name)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		slog.Error("备份失败 "+name, "error", err)
		return
	}
	f, err := os.Create(backupFile)
	if err != nil {
		slog.Error("备份失败 "+name, "error", err)
		return
	}
	err = writeBackup(f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		slog.Error("备份失败 "+name, "error", err)
		// 删除不完整的备份，避免当天不再重试
		os.Remove(backupFile)
		return
	}
	slog.Info("备份设置成功 " + name)
}

// WriteBackup 将设置打包写入 w
func WriteBackup(w io.Writer) error {
	backupMu.Lock()
	defer backupMu.Unlock()
	return writeBackup(w)
}

// writeBackup 清理残余封面后打包 files、torrents、订阅与设置文件，跳过隐藏文件，调用方需持有 backupMu
func writeBackup(w io.Writer) error {
	if _, err := store.ClearCover(); err != nil {
		slog.Error("清理残余封面失败", "error", err)
	}

	cfgDir := config.ConfigDir()
	zipWriter := zip.NewWriter(w)
	for _, name := range []string{"files", "torrents", store.AniFileName, config.FileName} {
		full := filepath.Join(cfgDir, name)
		info, err := os.Stat(full)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if err := utils.AddZipFile(zipWriter, full, name); err != nil {
				return err
			}
			continue
		}
		err = filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			rel, err := filepath.Rel(cfgDir, path)
			if err != nil {
				return err
			}
			return utils.AddZipFile(zipWriter, path, rel)
		})
		if err != nil {
			return err
		}
	}
	return zipWriter.Close()
}

// clearBackup 删除超过保留天数的 yyyy-MM-dd.zip 备份
func clearBackup(backupDir string, days int) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}
	expiration := time.Now().AddDate(0, 0, -days)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".zip" {
			continue
		}
		t, err := time.ParseInLocation(time.DateOnly, strings.TrimSuffix(entry.Name(), ".zip"), time.Local)
		if err != nil {
			slog.Error(err.Error())
			continue
		}
		if t.After(expiration) {
			continue
		}
		slog.Info(entry.Name() + " 备份已过期, 自动删除")
		if err := os.Remove(filepath.Join(backupDir, entry.Name())); err != nil {
			slog.Error(err.Error())
		}
	}
}

// ImportBackup 删除旧的种子记录，将备份解压到配置目录后重新加载设置与订阅
func ImportBackup(reader *zip.Reader) error {
	backupMu.Lock()
	defer backupMu.Unlock()
	cfgDir := config.ConfigDir()
	if err := os.RemoveAll(filepath.Join(cfgDir, "torrents")); err != nil {
		return err
	}
	if err := utils.Unzip(reader, cfgDir); err != nil {
		return err
	}
	if err := config.Load(); err != nil {
		return err
	}
	return store.LoadAniList()
}
