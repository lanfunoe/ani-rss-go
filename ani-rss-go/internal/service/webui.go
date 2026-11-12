// Package service — 上传或在线更新的 WebUI
package service

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/utils"
)

// errNoWebUIUpdate 未上传 WebUI 或已是最新版本
var errNoWebUIUpdate = errors.New("无 WebUI 更新")

// WebUIDir 上传的 WebUI 所在目录，其中的文件优先于内嵌前端
func WebUIDir() string {
	return filepath.Join(config.ConfigDir(), "webui")
}

// getWebUI 读取 WebUI 目录下的 webui.json，未上传 WebUI 时返回 nil
func getWebUI() (*entity.WebUI, error) {
	data, err := os.ReadFile(filepath.Join(WebUIDir(), "webui.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var webUI entity.WebUI
	if err := json.Unmarshal(data, &webUI); err != nil {
		return nil, err
	}
	return &webUI, nil
}

// WebUIUpdate 检测 webui.json 中声明的 GitHub 仓库是否有新版本
func WebUIUpdate() (*entity.UpdateInfo, error) {
	webUI, err := getWebUI()
	if err != nil {
		return nil, err
	}
	if webUI == nil {
		return nil, errNoWebUIUpdate
	}
	return githubUpdateInfo(webUI.Owner, webUI.Repo, webUI.Filename, webUI.Version)
}

// UpdateWebUI 下载新版本 WebUI，校验大小与 sha256 后替换 WebUI 目录
func UpdateWebUI() error {
	info, err := WebUIUpdate()
	if err != nil {
		return err
	}
	if !info.Update {
		return errNoWebUIUpdate
	}
	slog.Info("更新 WebUI")

	temp, err := os.CreateTemp("", "ani-rss-webui-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()

	resp, err := httpc.Get(info.DownloadUrl).Do()
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return err
	}
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(temp, hash), resp.Body)
	if err != nil {
		return err
	}
	if size != info.Size {
		return errors.New("WebUI 下载出现问题")
	}
	if hex.EncodeToString(hash.Sum(nil)) != info.Sha256 {
		return errors.New("WebUI 更新文件的 sha256 不匹配")
	}

	reader, err := zip.NewReader(temp, size)
	if err != nil {
		return err
	}
	if err := replaceWebUI(reader); err != nil {
		return err
	}
	slog.Info("WebUI 更新完成")
	return nil
}

// UploadWebUI 上传包含 webui.json 的 zip 并替换 WebUI 目录
func UploadWebUI(r io.ReaderAt, size int64, filename string) error {
	if strings.ToLower(filepath.Ext(filename)) != ".zip" {
		return errors.New("文件格式错误")
	}
	slog.Info("上传 WebUI " + filename)
	reader, err := zip.NewReader(r, size)
	if err != nil {
		return errors.New("上传 WebUI 失败")
	}
	if _, err := fs.Stat(reader, "webui.json"); err != nil {
		return errors.New("上传 WebUI 失败")
	}
	if err := replaceWebUI(reader); err != nil {
		return err
	}
	slog.Info("WebUI 上传完成")
	return nil
}

// DeleteWebUI 删除 WebUI 目录，恢复使用内嵌前端
func DeleteWebUI() error {
	if err := os.RemoveAll(WebUIDir()); err != nil {
		return err
	}
	slog.Info("已删除 WebUI")
	return nil
}

// replaceWebUI 清空 WebUI 目录后解压
func replaceWebUI(reader *zip.Reader) error {
	dir := WebUIDir()
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return utils.Unzip(reader, dir)
}
