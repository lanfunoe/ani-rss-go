package web

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/download"
	"ani-rss-go/internal/store"
)

// handleTorrentsInfos 下载列表
func handleTorrentsInfos(c *gin.Context) {
	ok(c, download.GetTorrentsInfos())
}

// handleDeleteTorrent 删除缓存的种子
func handleDeleteTorrent(c *gin.Context) {
	id, found := requiredParam(c, "id")
	if !found {
		return
	}
	hash, found := requiredParam(c, "hash")
	if !found {
		return
	}
	ani := store.FindAni(id)
	if ani == nil {
		fail(c, errors.New("此订阅不存在"))
		return
	}
	var hashList []string
	for h := range strings.SplitSeq(hash, ",") {
		h = strings.TrimSpace(h)
		if h != "" {
			hashList = append(hashList, h)
		}
	}
	store.DeleteTorrentFiles(ani, hashList)
	okMsg(c, nil, "删除完成")
}
