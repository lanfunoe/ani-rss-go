// Package service — 爱发电捐赠校验与捐赠缓存数据
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/utils"
)

// VerifyNo 校验爱发电订单号，成功时写入到期时间
func VerifyNo(no string) (*entity.Result, error) {
	if strings.TrimSpace(no) == "" {
		return nil, errors.New("订单号为空")
	}
	resp, err := httpc.Post("https://afdian.wushuo.top").
		AddQuery("out_trade_no", no).
		SetTimeout(5 * time.Second).
		Do()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return nil, err
	}
	out := struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Code == entity.CodeOK {
		cfg := config.Get()
		cfg.OutTradeNo = no
		cfg.ExpirationTime = new(time.Now().AddDate(999, 0, 0).UnixMilli())
		cfg.TryOut = new(false)
		config.Sync()
	}
	return &entity.Result{Code: out.Code, Message: out.Message, T: time.Now().UnixMilli()}, nil
}

// VerifyExpirationTime 捐赠是否有效
func VerifyExpirationTime() bool {
	return time.Now().UnixMilli() < utils.Deref(config.Get().ExpirationTime)
}

// mikanBgm Mikan 与 BGM 条目的映射
type mikanBgm struct {
	MikanId string  `json:"mikanId"`
	BgmId   string  `json:"bgmId"`
	Score   float64 `json:"score"`
}

// donateAllowed 是否允许获取捐赠缓存数据
func donateAllowed() bool {
	cfg := config.Get()
	return VerifyExpirationTime() && cfg.OutTradeNo != "" && !utils.Deref(cfg.TryOut)
}

// donateGet 请求捐赠缓存数据，404 视为空数据
func donateGet(path string, out any) {
	if !donateAllowed() {
		return
	}
	sum := sha256.Sum256([]byte(config.Get().OutTradeNo))
	resp, err := httpc.Get("https://cache.wushuo.top/" + path + "/" + hex.EncodeToString(sum[:])).
		SetTimeout(5 * time.Second).
		Do()
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return
	}
	if err := resp.AssertStatus(); err != nil {
		slog.Error(err.Error())
		return
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		slog.Error(err.Error())
	}
}

// donateMikanBgm 获取 Mikan ID 到 BGM 信息的映射
func donateMikanBgm() map[string]mikanBgm {
	m := map[string]mikanBgm{}
	donateGet("mikan/bgm", &m)
	return m
}

// donateScore 获取捐赠用户可用的评分缓存，source 为 bgm 或 mikan
func donateScore(source string) map[string]float64 {
	scores := map[string]float64{}
	donateGet(source+"/score", &scores)
	return scores
}
