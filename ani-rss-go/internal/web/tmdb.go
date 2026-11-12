package web

import (
	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/service"
)

func handleGetThemoviedbName(c *gin.Context) {
	in, okBind := bind[entity.TMDBNameRequest](c)
	if !okBind {
		return
	}
	result, err := service.GetTMDBName(in)
	if err != nil {
		fail(c, err)
		return
	}
	okMsg(c, result, "获取 TMDB 成功")
}

func handleGetThemoviedbGroup(c *gin.Context) {
	in, okBind := bind[entity.Ani](c)
	if !okBind {
		return
	}
	groups, err := service.GetTMDBGroups(in)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, groups)
}
