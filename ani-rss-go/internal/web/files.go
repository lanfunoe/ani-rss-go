package web

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/service"
	"ani-rss-go/internal/utils"
)

const maxImageBytes = 8 << 20

var errNotAllowed = errors.New("不允许访问")

// writeNotFound 写入 404 页面
func writeNotFound(c *gin.Context) {
	writeTextHTML(c, http.StatusNotFound, "404 Not Found !")
}

// handleFile 获取文件
func handleFile(c *gin.Context) {
	filename, found := requiredParam(c, "filename")
	if !found {
		return
	}
	name, err := decodeBase64Param(filename)
	if err != nil {
		fail(c, errNotAllowed)
		return
	}
	name = filepath.FromSlash(name)
	if filepath.IsAbs(name) {
		serveMediaFile(c, name)
		return
	}
	if !filepath.IsLocal(name) {
		fail(c, errNotAllowed)
		return
	}
	root, err := openImageRoot("files", false)
	if err != nil {
		writeNotFound(c)
		return
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		writeNotFound(c)
		return
	}
	defer file.Close()
	if err := serveImageFile(c, file); err != nil {
		fail(c, errNotAllowed)
	}
}

func handleUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImageBytes+(64<<10))
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		if c.Request.MultipartForm != nil {
			c.Request.MultipartForm.RemoveAll()
		}
		fail(c, fmt.Errorf("上传失败，文件不得超过 8 MiB: %w", err))
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	header, err := c.FormFile("file")
	if err != nil {
		fail(c, err)
		return
	}
	if header.Size > maxImageBytes {
		fail(c, errors.New("文件不得超过 8 MiB"))
		return
	}
	src, err := header.Open()
	if err != nil {
		fail(c, err)
		return
	}
	defer src.Close()
	data, err := readImageBytes(src)
	if err != nil {
		fail(c, err)
		return
	}
	_, ext := imageType(data)
	if ext == "" {
		fail(c, errors.New("仅允许上传栅格图片"))
		return
	}
	fileExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(header.Filename), "."))
	if fileExt == "jpeg" && ext == "jpg" {
		ext = fileExt
	} else if fileExt != ext {
		fail(c, errors.New("文件扩展名与图片格式不一致"))
		return
	}
	root, err := openImageRoot("files", true)
	if err != nil {
		fail(c, err)
		return
	}
	defer root.Close()
	sum := fmt.Sprintf("%x", md5.Sum(data))
	name := filepath.Join(sum[:1], sum+"."+ext)
	if err := root.MkdirAll(sum[:1], 0o755); err != nil {
		fail(c, err)
		return
	}
	if err := utils.WriteRootFileAtomic(root, name, data); err != nil {
		fail(c, err)
		return
	}
	okMsg(c, filepath.ToSlash(name), "上传完成")
}

// maxUploadBytes 上传并读取接口的文件大小上限
const maxUploadBytes = 200 << 20

// readUploadFile 读取表单中 file 字段的全部内容
func readUploadFile(c *gin.Context) ([]byte, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes+(64<<10))
	header, err := c.FormFile("file")
	if err != nil {
		return nil, err
	}
	src, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	return io.ReadAll(src)
}

// handleUploadAndReadToBase64 上传并读取为 base64
func handleUploadAndReadToBase64(c *gin.Context) {
	data, err := readUploadFile(c)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, base64.StdEncoding.EncodeToString(data))
}

// handleUploadAndRead 上传并读取为 UTF-8 文本
func handleUploadAndRead(c *gin.Context) {
	data, err := readUploadFile(c)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, string(data))
}

// handleProxyImage 下载并缓存图片
func handleProxyImage(c *gin.Context) {
	imgUrl, found := requiredParam(c, "imgUrl")
	if !found {
		return
	}
	rawURL, err := decodeBase64Param(imgUrl)
	if err != nil {
		fail(c, errors.New("图片地址参数无效"))
		return
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		fail(c, errors.New("图片地址必须为 HTTP 或 HTTPS 地址"))
		return
	}
	u.Fragment, u.RawFragment = "", ""
	root, err := openImageRoot("img", true)
	if err != nil {
		fail(c, errors.New("无法访问图片缓存"))
		return
	}
	defer root.Close()
	name := fmt.Sprintf("%x.img", sha256.Sum256([]byte(u.String())))
	if file, err := root.Open(name); err == nil {
		err = serveImageFile(c, file)
		file.Close()
		if err == nil {
			return
		}
	}
	resp, err := httpc.Get(u.String()).Do()
	if err != nil {
		fail(c, errors.New("图片请求失败"))
		return
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		fail(c, errors.New("图片服务器返回异常"))
		return
	}
	data, err := readImageBytes(resp.Body)
	if err != nil {
		fail(c, errors.New("图片读取失败或超过 8 MiB"))
		return
	}
	contentType, ext := imageType(data)
	if ext == "" {
		fail(c, errors.New("响应不是支持的图片格式"))
		return
	}
	if err := utils.WriteRootFileAtomic(root, name, data); err == nil {
		if file, err := root.Open(name); err == nil {
			err = serveImageFile(c, file)
			file.Close()
			if err == nil {
				return
			}
		}
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, max-age=2592000")
	c.Data(http.StatusOK, contentType, data)
}

func openImageRoot(name string, create bool) (*os.Root, error) {
	root, err := os.OpenRoot(config.ConfigDir())
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if create {
		if err := root.MkdirAll(name, 0o755); err != nil {
			return nil, err
		}
	}
	return root.OpenRoot(name)
}

func readImageBytes(src io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(src, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, errors.New("文件不得超过 8 MiB")
	}
	return data, nil
}

func serveImageFile(c *gin.Context, file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return os.ErrPermission
	}
	var header [512]byte
	n, err := file.ReadAt(header[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	contentType, ext := imageType(header[:n])
	if ext == "" {
		return errors.New("不支持的图片格式")
	}
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, max-age=2592000")
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filepath.Base(file.Name())}))
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
	return nil
}

// serveMediaFile 提供订阅下载目录内的视频与字幕，支持 Range 请求
func serveMediaFile(c *gin.Context, name string) {
	file, err := service.OpenMediaFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		writeNotFound(c)
		return
	}
	if err != nil {
		fail(c, errNotAllowed)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		fail(c, errNotAllowed)
		return
	}
	ext := strings.ToLower(filepath.Ext(name))
	contentType := mime.TypeByExtension(ext)
	switch {
	case ext == ".mkv":
		contentType = "video/x-matroska"
	case contentType == "" || utils.IsSubtitleFormat(name):
		contentType = "text/plain; charset=utf-8"
	}
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": info.Name()}))
	if !utils.IsVideoFormat(name) && info.Size() <= 3<<20 {
		// 小于或者等于 3M 缓存 30 天
		c.Header("Cache-Control", "private, max-age=2592000")
	}
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
}

// imageType 按内容识别允许上传与提供访问的栅格图片，返回 Content-Type 与扩展名，无法识别时均为空
func imageType(data []byte) (string, string) {
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/png":
		return contentType, "png"
	case "image/jpeg":
		return contentType, "jpg"
	case "image/webp":
		return contentType, "webp"
	case "image/gif":
		return contentType, "gif"
	case "image/bmp":
		return contentType, "bmp"
	case "image/x-icon":
		return contentType, "ico"
	case "image/avif":
		return contentType, "avif"
	default:
		return "", ""
	}
}
