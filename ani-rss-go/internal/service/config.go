// Package service — 设置：配置读写、代理测试与缓存清理
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"github.com/PuerkitoBio/goquery"

	"ani-rss-go/internal/autostart"
	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/store"
	"ani-rss-go/internal/utils"
)

// GetConfig 返回给前端的设置，不含密码摘要与令牌签名密钥
func GetConfig() map[string]any {
	cfg := config.Get()
	data, _ := json.Marshal(cfg)
	var out map[string]any
	json.Unmarshal(data, &out)
	// 不向前端泄露密码摘要（token 按真实值返回，前端整体回传，清空输入框即为清除）
	login := map[string]any{"username": "", "password": ""}
	if cfg.Login != nil {
		login["username"] = cfg.Login.Username
	}
	out["login"] = login
	// 令牌签名密钥不返回
	out["jwtKey"] = ""
	out["verifyExpirationTime"] = VerifyExpirationTime()
	return out
}

// SetConfig 合并并保存设置，返回是否需要重启循环任务
func SetConfig(body map[string]json.RawMessage) (bool, error) {
	cfg := config.Get()
	// 到期时间、订单号与令牌签名密钥不允许修改
	delete(body, "expirationTime")
	delete(body, "outTradeNo")
	delete(body, "tryOut")
	delete(body, "jwtKey")
	oldUsername := ""
	if cfg.Login != nil {
		oldUsername = cfg.Login.Username
	}
	// 用户名或密码为空表示未修改
	passwordChanged := false
	if v, exists := body["login"]; exists {
		var login entity.Login
		if json.Unmarshal(v, &login) == nil {
			passwordChanged = login.Password != ""
			if cfg.Login != nil {
				if login.Username == "" {
					login.Username = cfg.Login.Username
				}
				if login.Password == "" {
					login.Password = cfg.Login.Password
				}
			}
			if data, err := json.Marshal(login); err == nil {
				body["login"] = data
			}
		}
	}
	candidate := *cfg
	if err := config.Merge(&candidate, body); err != nil {
		return false, err
	}
	// 用户名或密码发生改动时更换 tokenId，使已登录的令牌失效
	if candidate.Login != nil && (candidate.Login.Username != oldUsername || passwordChanged) {
		candidate.TokenId = uuid.New().String()
	}
	// 代理参数校验
	if candidate.GetProxy() && (strings.TrimSpace(candidate.ProxyHost) == "" || utils.Deref(candidate.ProxyPort) < 1 || utils.Deref(candidate.ProxyPort) > 65535) {
		return false, errors.New("代理参数不完整")
	}
	// 开机自启发生改变，同步失败时不保存设置
	if candidate.GetAutoStart() != cfg.GetAutoStart() {
		if err := autostart.Sync(candidate.GetAutoStart()); err != nil {
			return false, err
		}
	}
	// 时间间隔发生改变时重启任务
	restart := utils.Deref(candidate.RssSleepMinutes) != utils.Deref(cfg.RssSleepMinutes) ||
		utils.Deref(candidate.RenameSleepSeconds) != utils.Deref(cfg.RenameSleepSeconds)
	*cfg = candidate
	config.Sync()
	return restart, nil
}

// ClearCache 清理封面缓存与进程内缓存，返回清理的总大小
func ClearCache() (string, error) {
	total, err := store.ClearCover()
	if err != nil {
		return "", err
	}

	// 清理 mikan 预览封面
	imgDir := filepath.Join(config.ConfigDir(), "img")
	if st, err := os.Stat(imgDir); err == nil && st.IsDir() {
		if imgSize, err := utils.DirSize(imgDir); err == nil {
			total += imgSize
		}
		os.RemoveAll(imgDir)
	}

	// 进程内缓存
	utils.Cache.DeleteAll()

	return utils.FormatSize(total, true), nil
}

// proxyTestResult 代理测试结果
type proxyTestResult struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Time   int64  `json:"time"`
}

// TestProxy 使用待测试配置中的代理请求目标地址
func TestProxy(target string, cfg *entity.Config) (any, error) {
	target = strings.TrimSpace(target)
	transport := &http.Transport{}
	if cfg.GetProxy() && cfg.ProxyHost != "" && utils.Deref(cfg.ProxyPort) > 0 {
		proxyURL := &url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("%s:%d", cfg.ProxyHost, *cfg.ProxyPort),
		}
		if cfg.ProxyUsername != "" && cfg.ProxyPassword != "" {
			proxyURL.User = url.UserPassword(cfg.ProxyUsername, cfg.ProxyPassword)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second}

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", httpc.UserAgent)
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return proxyTestResult{Time: time.Since(start).Milliseconds()}, err
	}
	defer resp.Body.Close()
	result := proxyTestResult{Status: resp.StatusCode}
	// 页面标题用于确认访问到的是目标站点
	if doc, err := goquery.NewDocumentFromReader(resp.Body); err == nil {
		result.Title = strings.Join(strings.Fields(doc.Find("title").First().Text()), " ")
	}
	result.Time = time.Since(start).Milliseconds()
	return result, nil
}
