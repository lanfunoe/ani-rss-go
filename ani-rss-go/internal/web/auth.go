package web

import (
	"cmp"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"uuid"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/utils"
)

// handleLogin 登录
func handleLogin(c *gin.Context) {
	login, okBind := bind[entity.Login](c)
	if !okBind {
		return
	}
	token, err := loginWithPassword(c.Request, login)
	if err != nil {
		if errors.Is(err, errLoginLimited) {
			c.JSON(http.StatusOK, entity.Forbid(err.Error()))
			return
		}
		fail(c, err)
		return
	}
	okMsg(c, token, "登录成功")
}

// handleTestIpWhitelist IP白名单测试
func handleTestIpWhitelist(c *gin.Context) {
	if ipWhitelisted(c.Request) {
		ok(c, nil)
		return
	}
	c.JSON(http.StatusOK, entity.Error("error"))
}

// errLoginLimited 登录失败次数过多
var errLoginLimited = errors.New("失败次数过多, 已限制登录")

var (
	sessionMu sync.Mutex
	// sessionID 当前会话 ID，空串表示进程启动后尚未初始化
	sessionID string
)

// newSessionId 禁止多设备登录时生成新的会话 ID，否则固定为 "-"
func newSessionId() string {
	if config.Get().GetMultiLoginForbidden() {
		return uuid.New().String()
	}
	return "-"
}

// resetSessionId 刷新会话 ID，禁止多设备登录时新登录会顶掉旧设备
func resetSessionId() string {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	sessionID = newSessionId()
	return sessionID
}

// currentSessionId 当前会话 ID，进程启动后首次使用时按配置初始化
func currentSessionId() string {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if sessionID == "" {
		sessionID = newSessionId()
	}
	return sessionID
}

// jwtKey 解码配置中的令牌签名密钥
func jwtKey(cfg *entity.Config) ([]byte, error) {
	return base64.StdEncoding.DecodeString(cfg.JwtKey)
}

// newToken 生成登录令牌，登录有效时长小于 1 小时则永不过期
func newToken(ip string) (string, error) {
	cfg := config.Get()
	key, err := jwtKey(cfg)
	if err != nil {
		return "", err
	}
	// tokenId 在修改用户名或密码后变更，使旧令牌失效
	claims := jwt.MapClaims{
		"sessionId": resetSessionId(),
		"tokenId":   cfg.TokenId,
		"ip":        ip,
	}
	if hours := utils.DerefOr(cfg.LoginEffectiveHours, 3); hours > 0 {
		claims["exp"] = jwt.NewNumericDate(time.Now().Add(time.Duration(hours) * time.Hour))
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
}

// verifyToken 校验登录令牌：签名、有效期、tokenId、会话 ID，开启 verifyLoginIp 时校验登录 IP
func verifyToken(token, ip string) bool {
	if token == "" {
		return false
	}
	cfg := config.Get()
	key, err := jwtKey(cfg)
	if err != nil {
		return false
	}
	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return false
	}
	if cfg.GetVerifyLoginIp() && claims["ip"] != ip {
		return false
	}
	if claims["tokenId"] != cfg.TokenId {
		// 可能已经修改用户名或密码
		return false
	}
	return claims["sessionId"] == currentSessionId()
}

// clientIP 获取客户端 IP（支持受信任反向代理）
func clientIP(r *http.Request) string {
	cfg := config.Get()
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if !utils.Deref(cfg.ReverseProxyTrustIpListEnabled) || !slices.Contains(cfg.ReverseProxyTrustIpList, ip) {
		return ip
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first, _, _ := strings.Cut(forwarded, ",")
		return strings.TrimSpace(first)
	}
	return ip
}

// authenticated 鉴权检测：API Key / IP 白名单 / Header / Form(?s=) 任一通过
func authenticated(r *http.Request) bool {
	ip := clientIP(r)
	if err := limitLoginAttempts(ip, false); err != nil {
		return false
	}
	cfg := config.Get()
	// api key 可放在请求头或查询参数
	apiKey := cfg.ApiKey != "" && slices.ContainsFunc([]string{"api-key", "x-api-key", "s"}, func(k string) bool {
		v := cmp.Or(r.Header.Get(k), r.URL.Query().Get(k))
		return subtle.ConstantTimeCompare([]byte(v), []byte(cfg.ApiKey)) == 1
	})
	if apiKey || checkIPWhitelist(ip, cfg) || verifyToken(r.Header.Get("Authorization"), ip) || verifyToken(r.URL.Query().Get("s"), ip) {
		return true
	}
	_ = limitLoginAttempts(ip, true)
	return false
}

// checkIPWhitelist 白名单每行支持单个 IP、通配符（192.168.*.1）、CIDR 与范围（起始-结束）
func checkIPWhitelist(ip string, cfg *entity.Config) bool {
	if !cfg.GetIpWhitelist() || cfg.IpWhitelistStr == "" || ip == "" {
		return false
	}
	// 解析失败时 addr 为零值，不会命中 CIDR 与范围
	addr, _ := netip.ParseAddr(ip)
	addr = addr.Unmap()
	for line := range strings.SplitSeq(cfg.IpWhitelistStr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == ip {
			return true
		}
		if strings.Contains(line, "*") && wildcardMatch(line, ip) {
			return true
		}
		if prefix, err := netip.ParsePrefix(line); err == nil && prefix.Contains(addr) {
			return true
		}
		if lo, hi, ok := strings.Cut(line, "-"); ok {
			start, err1 := netip.ParseAddr(strings.TrimSpace(lo))
			end, err2 := netip.ParseAddr(strings.TrimSpace(hi))
			if err1 == nil && err2 == nil && addr.IsValid() && start.Compare(addr) <= 0 && addr.Compare(end) <= 0 {
				return true
			}
		}
	}
	return false
}

// publicAccessForbidden 开启仅允许内网访问时，请求来源是否为公网
func publicAccessForbidden(r *http.Request) bool {
	if !config.Get().GetInnerIP() {
		return false
	}
	return !isInnerIP(clientIP(r))
}

// isInnerIP 是否为 IPv4 内网或本机地址，IPv6 视为公网
func isInnerIP(ip string) bool {
	addr := net.ParseIP(ip).To4()
	return addr != nil && (addr.IsPrivate() || addr.IsLoopback())
}

// ipWhitelisted 请求来源 IP 是否在白名单中
func ipWhitelisted(r *http.Request) bool {
	return checkIPWhitelist(clientIP(r), config.Get())
}

// wildcardMatch 按段匹配 IPv4 通配符，段可为 *、数值或范围（0-16）
func wildcardMatch(pattern, ip string) bool {
	pp := strings.Split(pattern, ".")
	ipn := strings.Split(ip, ".")
	if len(pp) != 4 || len(ipn) != 4 {
		return false
	}
	for i := range 4 {
		if pp[i] == "*" || pp[i] == ipn[i] {
			continue
		}
		// 单段范围，如 192.168.0-16.*
		lo, hi, ok := strings.Cut(pp[i], "-")
		if !ok {
			return false
		}
		start, err1 := strconv.Atoi(strings.TrimSpace(lo))
		end, err2 := strconv.Atoi(strings.TrimSpace(hi))
		v, err3 := strconv.Atoi(ipn[i])
		if err1 != nil || err2 != nil || err3 != nil || v < start || v > end {
			return false
		}
	}
	return true
}

// loginWithPassword 登录校验，返回令牌
func loginWithPassword(r *http.Request, myLogin *entity.Login) (string, error) {
	ip := clientIP(r)
	if err := limitLoginAttempts(ip, false); err != nil {
		return "", err
	}
	if myLogin.Username == "" {
		return "", errors.New("用户名不能为空")
	}
	if myLogin.Password == "" {
		return "", errors.New("密码不能为空")
	}
	cfg := config.Get()
	real := cfg.Login
	if real == nil || real.Username != myLogin.Username || real.Password != myLogin.Password {
		_ = limitLoginAttempts(ip, true)
		slog.Warn("登陆失败", "username", myLogin.Username, "ip", ip)
		time.Sleep(time.Duration(500+rand.IntN(4500)) * time.Millisecond)
		return "", errors.New("用户名或密码错误")
	}

	utils.Cache.Delete(limitKey(ip))
	slog.Info("登录成功", "username", myLogin.Username, "ip", ip)
	return newToken(ip)
}

func limitKey(ip string) string {
	return "LimitLoginAttempts#" + ip
}

// limitLoginAttempts 限制尝试次数（失败 30 次封禁一天），已限制时返回错误
func limitLoginAttempts(ip string, isAdd bool) error {
	cfg := config.Get()
	if !cfg.GetLimitLoginAttempts() {
		return nil
	}
	key := limitKey(ip)
	counter, ok := utils.CacheGet[*atomic.Int32](key)
	if !ok {
		if isAdd {
			var count atomic.Int32
			count.Add(1)
			utils.Cache.Set(key, &count, 24*time.Hour)
		}
		return nil
	}
	count := counter.Load()
	if isAdd {
		count = counter.Add(1)
	}
	// 失败时 时间将重新计时
	utils.Cache.Set(key, counter, 24*time.Hour)
	if count < 30 {
		return nil
	}
	slog.Debug("失败次数过多, 已限制登录", "ip", ip)
	return fmt.Errorf("%w %s", errLoginLimited, ip)
}
