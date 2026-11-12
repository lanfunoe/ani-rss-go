package web

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/service"
)

// dist 内嵌的前端构建产物，由 build.sh 从 ani-rss-ui/dist 复制到此目录
//
//go:embed all:dist
var dist embed.FS

// uiFS 前端资源：指定 webDir 时读取该目录，否则使用内嵌资源
func uiFS(webDir string) fs.FS {
	var ui fs.FS
	if webDir != "" {
		ui = os.DirFS(webDir)
	} else {
		ui, _ = fs.Sub(dist, "dist")
	}
	if _, err := fs.Stat(ui, "index.html"); err != nil {
		slog.Warn("未找到前端资源 index.html，请使用 build.sh 构建或通过 WEB_DIR 指定 ani-rss-ui 构建产物目录", "webDir", webDir)
	}
	return ui
}

// locate 返回首个包含文件 name 的资源目录，均不存在时返回 nil
func locate(layers []fs.FS, name string) fs.FS {
	for _, fsys := range layers {
		if info, err := fs.Stat(fsys, name); err == nil && !info.IsDir() {
			return fsys
		}
	}
	return nil
}

// serveStatic 返回前端资源，无扩展名路径回退到对应 html 或 index.html
// 上传的 WebUI 中存在的文件优先于 ui
func serveStatic(c *gin.Context, ui fs.FS) {
	layers := []fs.FS{os.DirFS(service.WebUIDir()), ui}
	name := strings.TrimPrefix(path.Clean(c.Request.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	ext := path.Ext(name)
	if ext == "" {
		name += ".html"
		if locate(layers, name) == nil {
			name = "index.html"
		}
	}
	fsys := locate(layers, name)
	if fsys == nil {
		c.String(http.StatusNotFound, "404 Not Found !")
		return
	}
	switch ext {
	case ".css", ".js", ".jpg", ".png", ".svg", ".ico":
		c.Header("Cache-Control", "private, max-age=86400")
	default:
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	}
	// 客户端支持 gzip 且存在构建时预压缩的文件时直接返回
	if info, err := fs.Stat(fsys, name+".gz"); err == nil && !info.IsDir() {
		c.Header("Vary", "Accept-Encoding")
		if strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			c.Header("Content-Encoding", "gzip")
			c.Header("Content-Type", mime.TypeByExtension(path.Ext(name)))
			name += ".gz"
		}
	}
	http.ServeFileFS(c.Writer, c.Request, fsys, name)
}

// textTemplate 提示页面
var textTemplate = template.Must(template.New("text").Parse(`<html lang="zh">
<head>
    <meta charset="UTF-8"/>
    <link rel="icon" href="/favicon.ico"/>
    <title>ANI-RSS</title>
    <meta name="robots" content="noindex">
    <meta name="description" content="{{.}}"/>
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=0">
</head>
<body>
<div class="container">
    <h2>{{.}}</h2>
</div>
</body>
<style>
    .container {
        width: 100%;
        height: 100%;
        display: flex;
        align-items: center;
        justify-content: center;
    }

    .dark {
        color: rgb(207, 211, 220);
        background-color: rgb(45, 46, 47);
    }
</style>
<script>
    let changeToDark = () => {
        document.documentElement.classList.toggle('dark');
    }

    (() => {
        let color = localStorage.getItem('vueuse-color-scheme');
        if (!color) {
            return
        }

        if (color === 'auto') {
            const isDarkMode = window.matchMedia('(prefers-color-scheme: dark)').matches;
            if (isDarkMode) {
                changeToDark();
            }
            return;
        }

        if (color === 'dark') {
            changeToDark();
        }
    })()
</script>
</html>
`))

// writeTextHTML 返回显示一行提示文字的页面
func writeTextHTML(c *gin.Context, status int, text string) {
	var buf bytes.Buffer
	if err := textTemplate.Execute(&buf, text); err != nil {
		c.String(status, text)
		return
	}
	c.Data(status, "text/html; charset=utf-8", buf.Bytes())
}
