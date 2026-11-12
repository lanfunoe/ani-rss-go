package web

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/service"
)

// handleVerifyNo 校验爱发电订单号
func handleVerifyNo(c *gin.Context) {
	in, okBind := bind[entity.Config](c)
	if !okBind {
		return
	}
	res, err := service.VerifyNo(in.OutTradeNo)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
