package web

import (
	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/service"
)

// handleRate 获取评分
func handleRate(c *gin.Context) {
	in, okBind := bind[entity.Ani](c)
	if !okBind {
		return
	}
	rate, err := service.BgmRate(service.GetSubjectId(in), nil)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, rate)
}

// handleSetRate 进行评分
func handleSetRate(c *gin.Context) {
	in, okBind := bind[entity.Ani](c)
	if !okBind {
		return
	}
	rate, err := service.BgmRate(service.GetSubjectId(in), new(int(in.Score)))
	if err != nil {
		fail(c, err)
		return
	}
	okMsg(c, rate, "保存评分成功")
}

// handleMeBgm 获取当前 BGM 账号信息
func handleMeBgm(c *gin.Context) {
	me, err := service.BgmMeWithExpires()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, me)
}

// handleBgmOAuthCallback BGM 授权回调
func handleBgmOAuthCallback(c *gin.Context) {
	code, found := requiredParam(c, "code")
	if !found {
		return
	}
	if err := service.BgmOAuthCallback(code); err != nil {
		fail(c, err)
		return
	}
	okMsg(c, nil, "授权成功, 现在你可以关闭此窗口")
}

// handleSearchBgm 搜索BGM条目
func handleSearchBgm(c *gin.Context) {
	name, found := requiredParam(c, "name")
	if !found {
		return
	}
	ok(c, service.SearchBgm(name))
}

// handleGetAniBySubjectId 将指定id的BGM番剧转换为订阅
func handleGetAniBySubjectId(c *gin.Context) {
	id, found := requiredParam(c, "id")
	if !found {
		return
	}
	ok(c, service.GetAniBySubjectId(id))
}

// handleGetBgmTitle 获取BGM标题
func handleGetBgmTitle(c *gin.Context) {
	in, okBind := bind[entity.Ani](c)
	if !okBind {
		return
	}
	title, err := service.GetBgmTitle(in)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, title)
}
