// ani-rss-go：基于 RSS 自动追番、订阅、下载、重命名
package main

import (
	"cmp"
	"errors"
	"flag"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/joho/godotenv"
	"gopkg.in/natefinch/lumberjack.v2"

	"ani-rss-go/internal/buildinfo"
	"ani-rss-go/internal/config"
	"ani-rss-go/internal/logsink"
	"ani-rss-go/internal/service"
	"ani-rss-go/internal/store"
	"ani-rss-go/internal/task"
	"ani-rss-go/internal/web"
)

func main() {
	// 须在读取任何环境变量之前加载
	loadDotEnv()

	port := flag.String("port", cmp.Or(os.Getenv("PORT"), "7789"), "监听端口")
	webDir := flag.String("web", os.Getenv("WEB_DIR"), "前端静态资源目录，为空时使用内嵌前端")
	flag.Parse()

	// 初始化日志（滚动写入 config/logs/ani-rss.log）
	setupLog()

	// 加载配置与订阅
	if err := config.Load(); err != nil {
		slog.Error("加载配置失败", "error", err)
		os.Exit(1)
	}
	initVersion()
	if err := store.LoadAniList(); err != nil {
		slog.Error("加载订阅失败", "error", err)
		os.Exit(1)
	}
	// 备份前会清理未被订阅使用的封面，需在订阅加载后执行
	service.Backup()

	task.Start()
	slog.Info("version", "v", buildinfo.Version)

	server := web.NewServer(*port, *webDir)
	slog.Info("http://127.0.0.1:" + *port)
	if err := server.Run(); err != nil {
		slog.Error("HTTP 服务启动失败", "error", err)
		os.Exit(1)
	}
}

// loadDotEnv 读取工作目录下的 .env 写入环境变量，不覆盖已存在的变量，
// 因此优先级为：命令行参数 > 环境变量 > .env > 默认值。文件不存在时跳过。
func loadDotEnv() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("读取 .env 失败", "error", err)
	}
}

func setupLog() {
	logDir := filepath.Join(config.ConfigDir(), "logs")
	_ = os.MkdirAll(logDir, 0o755)
	file := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "ani-rss.log"),
		MaxSize:    10,
		MaxBackups: 5,
		Compress:   true,
	}
	writer := io.MultiWriter(os.Stdout, file)
	slog.SetDefault(slog.New(logsink.New(slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))))
}

// initVersion 填充版本与构建信息，优先使用 -ldflags 注入值
func initVersion() {
	cfg := config.Get()
	cfg.Version = buildinfo.Version
	commit := buildinfo.Commit
	if commit == "" {
		commit = "unknown"
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					commit = s.Value
				}
			}
		}
	}
	cfg.GitInfo = map[string]string{
		"branch":        cmp.Or(buildinfo.Branch, "dev"),
		"commitId":      commit,
		"shortCommitId": commit[:min(len(commit), 7)],
	}
}
