// Package buildinfo 构建信息，发布构建通过 -ldflags "-X ani-rss-go/internal/buildinfo.<变量>=<值>" 注入
package buildinfo

var (
	// Version 版本号，未注入时为 dev
	Version = "dev"
	// Commit 提交 ID，未注入时使用 Go 构建信息中的 vcs.revision
	Commit = ""
	// Branch 分支名
	Branch = ""
)
