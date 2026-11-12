// Package utils 通用工具：文件操作、指针取值、拼音与进程内缓存
package utils

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"uuid"
)

// 视频格式
var videoFormat = map[string]bool{"mp4": true, "mkv": true, "avi": true, "wmv": true}

// 字幕格式
var subtitleFormat = map[string]bool{"ass": true, "ssa": true, "sub": true, "srt": true, "lyc": true, "sup": true, "pgs": true, "mks": true}

// 图片格式
var imageFormat = map[string]bool{"png": true, "jpg": true, "jpeg": true, "webp": true, "svg": true}

// IsVideoFormat 判断文件名是否为视频
func IsVideoFormat(name string) bool {
	return isFormat(name, videoFormat)
}

// IsSubtitleFormat 判断文件名是否为字幕
func IsSubtitleFormat(name string) bool {
	return isFormat(name, subtitleFormat)
}

func isFormat(name string, exts map[string]bool) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	name = strings.ToLower(name)
	if _, ext, ok := strings.CutLast(name, "."); ok && ext != "" {
		return exts[ext]
	}
	return exts[name]
}

// AbsPath 转为绝对路径并规范化：\ 转为 /，去除重复分隔符、. 与 ..，去除结尾的 /
// Unix 绝对路径与 Windows 盘符路径不拼接工作目录，空串原样返回
func AbsPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	if !strings.HasPrefix(p, "/") && !(len(p) >= 2 && p[1] == ':') {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
	}
	return strings.TrimRight(path.Clean(strings.ReplaceAll(p, `\`, "/")), "/")
}

// CopyFile 复制文件，目标文件已存在时覆盖
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}

// Move 移动文件或文件夹，覆盖已存在的同名文件；无法直接重命名（如跨文件系统）时复制后删除源
func Move(src, dst string) error {
	renameErr := os.Rename(src, dst)
	if renameErr == nil {
		return nil
	}
	info, err := os.Stat(src)
	if err != nil {
		return renameErr
	}
	if info.IsDir() {
		err = os.CopyFS(dst, os.DirFS(src))
	} else {
		err = CopyFile(src, dst)
	}
	if err != nil {
		return errors.Join(renameErr, err)
	}
	return os.RemoveAll(src)
}

// MoveEntries 将 src 目录下的文件与文件夹逐个移动到 dst 目录，单个失败时记录日志并继续
func MoveEntries(src, dst string) {
	entries, _ := os.ReadDir(src)
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		slog.Info(fmt.Sprintf("移动 %s ==> %s", from, to))
		if err := Move(from, to); err != nil {
			slog.Error("移动失败", "from", from, "to", to, "error", err)
		}
	}
}

// ClearDir 删除视为空的文件夹，并继续检查上一级（共两级）；不存在的文件夹视为空
func ClearDir(dir string) {
	for range 2 {
		if dir == "" || filepath.Dir(dir) == dir || !isEmptyDir(dir) {
			return
		}
		if _, err := os.Stat(dir); err == nil {
			slog.Info("清理空文件夹 " + dir)
			if err := os.RemoveAll(dir); err != nil {
				slog.Error(err.Error())
				return
			}
		}
		dir = filepath.Dir(dir)
	}
}

// isEmptyDir 文件夹内没有子文件夹，且只有图片、nfo 与系统生成的文件
func isEmptyDir(dir string) bool {
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			return false
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
		if imageFormat[ext] || ext == "nfo" {
			continue
		}
		if slices.ContainsFunc([]string{".DS_Store", "Thumbs.db", "bangumi.ini"}, func(s string) bool { return strings.EqualFold(s, name) }) {
			continue
		}
		return false
	}
	return true
}

// FormatSize 格式化文件大小，超过 TB/TiB 后不再换级
func FormatSize(size int64, use1024 bool) string {
	base := 1000.0
	units := []string{"B", "KB", "MB", "GB", "TB"}
	if use1024 {
		base = 1024.0
		units = []string{"B", "KiB", "MiB", "GiB", "TiB"}
	}
	value := float64(size)
	index := 0
	for value >= base && index < len(units)-1 {
		value /= base
		index++
	}
	return fmt.Sprintf("%.2f %s", value, units[index])
}

// WriteFileAtomic 先写入同目录临时文件并落盘，再原子替换目标文件，失败时目标文件保持不变
func WriteFileAtomic(name string, data []byte, perm os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".*.tmp")
	if err != nil {
		return err
	}
	// 替换成功后临时文件已不存在，删除失败可忽略
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err = errors.Join(err, f.Sync(), f.Close()); err != nil {
		return err
	}
	if err := os.Chmod(f.Name(), perm); err != nil {
		return err
	}
	return os.Rename(f.Name(), name)
}

// AddZipFile 向压缩包写入单个文件，name 为空时使用文件名
func AddZipFile(zipWriter *zip.Writer, path, name string) error {
	if name == "" {
		name = filepath.Base(path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = filepath.ToSlash(name)
	hdr.Method = zip.Deflate
	w, err := zipWriter.CreateHeader(hdr)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// Unzip 将压缩包解压到 dir，跳过不在 dir 内的条目
func Unzip(reader *zip.Reader, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, zf := range reader.File {
		name := filepath.FromSlash(zf.Name)
		if !filepath.IsLocal(name) {
			continue
		}
		if zf.FileInfo().IsDir() {
			if err := root.MkdirAll(name, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := root.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		if err := unzipFile(root, zf, name); err != nil {
			return err
		}
	}
	return nil
}

// unzipFile 解压单个条目到 root 下的 name
func unzipFile(root *os.Root, zf *zip.File, name string) error {
	src, err := zf.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return errors.Join(err, dst.Close())
}

// DirSize 统计目录下全部文件的大小
func DirSize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if st, e := d.Info(); e == nil {
				total += st.Size()
			}
		}
		return nil
	})
	return total, err
}

// Exists 路径是否存在
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// WriteRootFileAtomic 在 root 内先写入同目录临时文件，再原子替换目标文件
func WriteRootFileAtomic(root *os.Root, name string, data []byte) error {
	temp := filepath.Join(filepath.Dir(name), "."+uuid.New().String()+".tmp")
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return root.Rename(temp, name)
}
