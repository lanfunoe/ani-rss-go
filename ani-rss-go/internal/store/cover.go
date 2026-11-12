package store

import (
	"crypto/md5"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/httpc"
)

// coverDir 封面与上传图片所在的 files 目录
func coverDir() string {
	return filepath.Join(config.ConfigDir(), "files")
}

// CoverName 封面在 files 目录下的相对路径：<md5 首字符>/<md5>.<扩展名>，扩展名取自地址路径，缺失时为 jpg
func CoverName(coverURL string) string {
	ext := "jpg"
	if u, err := url.Parse(coverURL); err == nil {
		if e := strings.TrimPrefix(path.Ext(u.Path), "."); e != "" {
			ext = e
		}
	}
	filename := fmt.Sprintf("%x", md5.Sum([]byte(coverURL))) + "." + ext
	return filename[:1] + "/" + filename
}

// defaultCover 默认封面
//
//go:embed cover.png
var defaultCover []byte

// SaveCover 下载封面到本地，返回相对 files 目录的路径，地址为空或下载失败时返回默认封面 cover.png
func SaveCover(coverURL string) string {
	filesDir := coverDir()
	_ = os.MkdirAll(filesDir, 0o755)

	cover := "cover.png"
	defaultFile := filepath.Join(filesDir, cover)
	if _, err := os.Stat(defaultFile); errors.Is(err, fs.ErrNotExist) {
		_ = os.WriteFile(defaultFile, defaultCover, 0o644)
	}
	if coverURL == "" {
		return cover
	}
	name := CoverName(coverURL)
	file := filepath.Join(filesDir, filepath.FromSlash(name))
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	if _, err := os.Stat(file); err == nil {
		return name
	}
	resp, err := httpc.Get(coverURL).Do()
	if err != nil {
		slog.Error("下载封面失败", "url", coverURL, "error", err)
		return cover
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		slog.Error("下载封面失败", "url", coverURL, "error", err)
		return cover
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return cover
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return cover
	}
	return name
}

// CoverFile 封面文件的绝对路径，文件不存在时返回空串
func CoverFile(cover string) string {
	if cover == "" {
		return ""
	}
	p := filepath.Join(coverDir(), cover)
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// RefreshCover 删除已缓存的封面后重新下载，返回新的封面路径，图片地址为空时使用默认封面
func RefreshCover(image string) (string, error) {
	if image != "" {
		file := filepath.Join(coverDir(), filepath.FromSlash(CoverName(image)))
		if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return SaveCover(image), nil
}

// ClearCover 删除未被订阅使用的残余封面，返回删除的大小
func ClearCover() (int64, error) {
	filesDir := coverDir()
	var total int64

	// 订阅正在使用的封面不删除
	keep := map[string]bool{}
	for _, ani := range ANIList() {
		if ani.Cover != "" {
			keep[ani.Cover] = true
		}
	}

	entries, err := os.ReadDir(filesDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, err
	}
	for _, dir := range entries {
		if !dir.IsDir() {
			continue
		}
		subs, err := os.ReadDir(filepath.Join(filesDir, dir.Name()))
		if err != nil {
			continue
		}
		for _, f := range subs {
			if f.IsDir() {
				continue
			}
			rel := dir.Name() + "/" + f.Name()
			if keep[rel] {
				continue
			}
			full := filepath.Join(filesDir, dir.Name(), f.Name())
			if st, e := f.Info(); e == nil {
				total += st.Size()
			}
			os.Remove(full)
		}
		// 清理空文件夹
		if rest, _ := os.ReadDir(filepath.Join(filesDir, dir.Name())); len(rest) == 0 {
			os.Remove(filepath.Join(filesDir, dir.Name()))
		}
	}
	return total, nil
}
