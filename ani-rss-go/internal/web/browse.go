package web

import (
	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/service"
)

// handleAniBT AniBT 番剧列表
func handleAniBT(c *gin.Context) {
	in, okBind := bind[service.AniBTQuery](c)
	if !okBind {
		return
	}
	data, err := service.AniBTList(in)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, data)
}

// handleAniBTGroup AniBT 番剧字幕组列表
func handleAniBTGroup(c *gin.Context) {
	bgmId, found := requiredParam(c, "bgmId")
	if !found {
		return
	}
	data, err := service.AniBTGroups(bgmId)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, data)
}

// handleAnimeGardenList AnimeGarden 番剧列表
func handleAnimeGardenList(c *gin.Context) {
	bgmUrl, _ := requestParam(c, "bgmUrl")
	data, err := service.AnimeGardenList(bgmUrl)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, data)
}

// handleAnimeGardenGroup AnimeGarden 番剧字幕组列表
func handleAnimeGardenGroup(c *gin.Context) {
	bgmId, found := requiredParam(c, "bgmId")
	if !found {
		return
	}
	data, err := service.AnimeGardenGroups(bgmId)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, data)
}
