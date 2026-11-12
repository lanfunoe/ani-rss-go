package web

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/service"
	"ani-rss-go/internal/store"
	"ani-rss-go/internal/task"
)

// handleListAni 订阅列表
func handleListAni(c *gin.Context) {
	ok(c, service.ListAni())
}

// handleAddAni 添加订阅，override=true 时覆盖已存在的订阅
func handleAddAni(c *gin.Context) {
	in, okBind := bind[entity.Ani](c)
	if !okBind {
		return
	}
	override, valid := boolParam(c, "override", false)
	if !valid {
		return
	}
	if err := service.AddAni(in, override); err != nil {
		fail(c, err)
		return
	}
	okMsg(c, nil, "添加订阅成功")
}

// handleSetAni 修改订阅
func handleSetAni(c *gin.Context) {
	in, okBind := bind[entity.Ani](c)
	if !okBind {
		return
	}
	move, _ := requestParam(c, "move")
	if err := service.SetAni(in, strings.EqualFold(move, "true")); err != nil {
		fail(c, err)
		return
	}
	okMsg(c, nil, "修改成功")
}

// handleDeleteAni 删除订阅
func handleDeleteAni(c *gin.Context) {
	ids, okBind := bind[[]string](c)
	if !okBind {
		return
	}
	deleteFiles, valid := boolParam(c, "deleteFiles", true)
	if !valid {
		return
	}
	if err := service.DeleteAni(*ids, deleteFiles); err != nil {
		fail(c, err)
		return
	}
	okMsg(c, nil, "删除订阅成功")
}

// handlePreviewAni 预览订阅
func handlePreviewAni(c *gin.Context) {
	in, okBind := bind[entity.Ani](c)
	if !okBind {
		return
	}
	data, err := service.PreviewAni(in)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, data)
}

// handleDownloadPath 获取订阅的下载位置
func handleDownloadPath(c *gin.Context) {
	in, okBind := bind[entity.Ani](c)
	if !okBind {
		return
	}
	ok(c, service.DownloadPathPreview(in))
}

// handleRefreshAni 刷新单个订阅
func handleRefreshAni(c *gin.Context) {
	in, okBind := bind[struct {
		ID string `json:"id"`
	}](c)
	if !okBind {
		return
	}
	ani := store.FindAni(in.ID)
	if ani == nil {
		fail(c, errors.New("订阅不存在"))
		return
	}
	if err := task.SyncLock(true); err != nil {
		fail(c, err)
		return
	}
	go func() {
		defer task.Unlock()
		task.Download(context.Background(), []*entity.Ani{ani})
	}()
	okMsg(c, nil, "已开始刷新RSS")
}

// handleRefreshAll 刷新全部订阅
func handleRefreshAll(c *gin.Context) {
	if err := task.SyncLock(true); err != nil {
		fail(c, err)
		return
	}
	go func() {
		defer task.Unlock()
		task.Download(context.Background(), store.ANIList())
	}()
	okMsg(c, nil, "已开始刷新RSS")
}

// handleBatchEnable 批量启用停用
func handleBatchEnable(c *gin.Context) {
	value, valid := boolParam(c, "value", true)
	if !valid {
		return
	}
	ids, okBind := bind[[]string](c)
	if !okBind {
		return
	}
	if err := service.BatchEnable(value, *ids); err != nil {
		fail(c, err)
		return
	}
	okMsg(c, nil, "修改完成")
}

// handleUpdateTotalEpisodeNumber 更新总集数
func handleUpdateTotalEpisodeNumber(c *gin.Context) {
	force, valid := boolParam(c, "force", true)
	if !valid {
		return
	}
	ids, okBind := bind[[]string](c)
	if !okBind {
		return
	}
	if len(*ids) == 0 {
		fail(c, errors.New("未选择订阅"))
		return
	}
	go service.UpdateTotalEpisodeNumbers(*ids, force)
	okMsg(c, nil, "已开始更新总集数")
}

// handleRssToAni 将RSS转换为订阅
func handleRssToAni(c *gin.Context) {
	in, okBind := bind[struct {
		URL            string              `json:"url"`
		Type           string              `json:"type"`
		BgmUrl         string              `json:"bgmUrl"`
		Subgroup       string              `json:"subgroup"`
		Enable         *bool               `json:"enable"`
		StandbyRssList []entity.StandbyRss `json:"standbyRssList"`
	}](c)
	if !okBind {
		return
	}
	ani, err := service.GetAniFromRss(&entity.Ani{
		URL:            in.URL,
		Type:           in.Type,
		BgmUrl:         in.BgmUrl,
		Subgroup:       in.Subgroup,
		Enable:         in.Enable,
		StandbyRssList: in.StandbyRssList,
	})
	if err != nil {
		fail(c, fmt.Errorf("RSS解析失败 %s", err.Error()))
		return
	}
	ok(c, ani)
}

// handleImportAni 导入订阅
func handleImportAni(c *gin.Context) {
	in, okBind := bind[struct {
		Filename string        `json:"filename"`
		AniList  []*entity.Ani `json:"aniList"`
		Conflict string        `json:"conflict"`
	}](c)
	if !okBind {
		return
	}
	if err := service.ImportAni(in.AniList, in.Conflict); err != nil {
		fail(c, err)
		return
	}
	okMsg(c, nil, "导入成功")
}

// handleRefreshCover 刷新封面
func handleRefreshCover(c *gin.Context) {
	in, okBind := bind[entity.Ani](c)
	if !okBind {
		return
	}
	cover, err := store.RefreshCover(in.Image)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, cover)
}
