package web

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/service"
	"ani-rss-go/internal/store"
)

// handleScrape 刮削
func handleScrape(c *gin.Context) {
	force, valid := boolParam(c, "force", true)
	if !valid {
		return
	}
	in, okBind := bind[entity.Ani](c)
	if !okBind {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("刮削异常", "title", in.Title, "error", r, "stack", string(debug.Stack()))
			}
		}()
		service.Scrape(in, force)
	}()
	okMsg(c, nil, fmt.Sprintf("已开始刮削 %s", in.Title))
}

// handleBatchScrape 批量刮削
func handleBatchScrape(c *gin.Context) {
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
	var anis []*entity.Ani
	for _, ani := range store.ANIList() {
		if slices.Contains(*ids, ani.ID) {
			anis = append(anis, ani)
		}
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("批量刮削异常", "error", r, "stack", string(debug.Stack()))
			}
		}()
		slog.Info("开始批量刮削...")
		for _, ani := range anis {
			service.Scrape(ani, force)
		}
		slog.Info("批量刮削完毕")
	}()
	okMsg(c, nil, fmt.Sprintf("已开始刮削%d个订阅", len(anis)))
}
