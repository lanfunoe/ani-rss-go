// Package httpc 统一 HTTP 客户端
// 特性：全局 Cookie、统一 UA、按域名分流代理（proxyList）
package httpc

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ani-rss-go/internal/buildinfo"
)

// UserAgent 统一 UA
var UserAgent = fmt.Sprintf("ani-rss/%s (https://github.com/wushuo894/ani-rss)", buildinfo.Version)

// ProxyConfig 代理配置由外部注入，避免循环依赖
type ProxyConfig struct {
	Enable    bool
	Host      string
	Port      int
	Username  string
	Password  string
	ProxyList string // 每行一个域名，命中才走代理
}

var (
	proxyMu    sync.RWMutex
	proxyURL   *url.URL
	proxyHosts []string
)

// client 全局 HTTP 客户端，Cookie 在请求间共享，重定向最多跟随 10 次
var client = newClient()

func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = proxyFor
	return &http.Client{Jar: jar, Transport: transport, Timeout: 20 * time.Second}
}

// SetProxyConfig 更新全局代理配置（host 与 port 均有效才启用）
func SetProxyConfig(c ProxyConfig) {
	var u *url.URL
	var hosts []string
	if c.Enable && c.Host != "" && c.Port > 0 {
		u = &url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", c.Host, c.Port)}
		// 用户名与密码需同时配置
		if c.Username != "" && c.Password != "" {
			u.User = url.UserPassword(c.Username, c.Password)
		}
		for s := range strings.SplitSeq(c.ProxyList, "\n") {
			if s = strings.TrimSpace(s); s != "" {
				hosts = append(hosts, s)
			}
		}
	}
	proxyMu.Lock()
	defer proxyMu.Unlock()
	proxyURL, proxyHosts = u, hosts
}

// proxyFor 按域名分流代理，命中 proxyList 才走配置的代理，否则使用环境变量代理
func proxyFor(req *http.Request) (*url.URL, error) {
	proxyMu.RLock()
	defer proxyMu.RUnlock()
	host := req.URL.Hostname()
	for _, s := range proxyHosts {
		if s == host || strings.HasSuffix(host, "."+s) {
			return proxyURL, nil
		}
	}
	return http.ProxyFromEnvironment(req)
}

// Request 请求构建器
type Request struct {
	Method string
	URL    string
	Header map[string]string
	Query  url.Values
	Form   url.Values
	Body   []byte
	// BodyReader 流式请求体，长度为 BodySize，用于上传大文件
	BodyReader io.Reader
	BodySize   int64
	// Timeout 请求总超时，0 使用默认的 20 秒，小于 0 不限制
	Timeout time.Duration
	Files   []FileField
}

// FileField multipart 文件字段
type FileField struct {
	Field    string
	FileName string
	Path     string
	Raw      []byte
}

// New 创建指定方法的请求
func New(method, u string) *Request {
	return &Request{Method: method, URL: u, Header: map[string]string{}, Query: url.Values{}}
}

func Get(u string) *Request  { return New(http.MethodGet, u) }
func Post(u string) *Request { return New(http.MethodPost, u) }
func Put(u string) *Request  { return New(http.MethodPut, u) }

func (r *Request) Set(key, value string) *Request {
	r.Header[key] = value
	return r
}

func (r *Request) AddQuery(key, value string) *Request {
	r.Query.Add(key, value)
	return r
}

func (r *Request) SetForm(key, value string) *Request {
	if r.Form == nil {
		r.Form = url.Values{}
	}
	r.Form.Add(key, value)
	return r
}

func (r *Request) SetBody(b []byte) *Request {
	r.Body = b
	return r
}

// SetBodyReader 设置流式请求体，size 为请求体长度
func (r *Request) SetBodyReader(body io.Reader, size int64) *Request {
	r.BodyReader, r.BodySize = body, size
	return r
}

// SetTimeout 设置请求总超时，小于 0 时不限制
func (r *Request) SetTimeout(d time.Duration) *Request {
	r.Timeout = d
	return r
}

func (r *Request) AddFile(field, path string) *Request {
	r.Files = append(r.Files, FileField{Field: field, FileName: filepath.Base(path), Path: path})
	return r
}

func (r *Request) AddFileBytes(field, fileName string, raw []byte) *Request {
	r.Files = append(r.Files, FileField{Field: field, FileName: fileName, Raw: raw})
	return r
}

// Response 响应封装
type Response struct {
	*http.Response
}

func (resp *Response) BodyBytes() ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (resp *Response) BodyString() (string, error) {
	b, err := resp.BodyBytes()
	return string(b), err
}

// Do 执行请求（自动 UA、去重斜杠、代理）
func (r *Request) Do() (*Response, error) {
	// 去除分隔符重复
	target, err := url.Parse(r.URL)
	if err != nil {
		return nil, err
	}
	path := target.EscapedPath()
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	target.Path, err = url.PathUnescape(path)
	if err != nil {
		return nil, err
	}
	target.RawPath = path
	if len(r.Query) > 0 {
		if target.RawQuery != "" {
			target.RawQuery += "&"
		}
		target.RawQuery += r.Query.Encode()
	}
	u := target.String()

	var body io.Reader
	if len(r.Files) > 0 {
		buf := &bytes.Buffer{}
		contentType, err := writeMultipart(buf, r.Files, r.Form)
		if err != nil {
			return nil, err
		}
		r.Header["Content-Type"] = contentType
		body = buf
	} else if r.Form != nil {
		r.Header["Content-Type"] = "application/x-www-form-urlencoded"
		body = strings.NewReader(r.Form.Encode())
	} else if r.BodyReader != nil {
		body = r.BodyReader
	} else if r.Body != nil {
		body = bytes.NewReader(r.Body)
	}

	req, err := http.NewRequest(r.Method, u, body)
	if err != nil {
		return nil, err
	}
	if r.BodyReader != nil {
		req.ContentLength = r.BodySize
	}
	for k, v := range r.Header {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", UserAgent)
	}

	c := client
	if r.Timeout != 0 {
		withTimeout := *client
		withTimeout.Timeout = max(r.Timeout, 0)
		c = &withTimeout
	}
	resp, err := c.Do(req)
	if err != nil {
		slog.Error("请求失败", "url", u, "error", err)
		return nil, err
	}
	return &Response{Response: resp}, nil
}

// writeMultipart 写入 multipart 表单（文件字段与普通字段），返回 Content-Type
func writeMultipart(w io.Writer, files []FileField, form url.Values) (string, error) {
	writer := multipart.NewWriter(w)
	for _, f := range files {
		data := f.Raw
		if f.Path != "" {
			var err error
			if data, err = os.ReadFile(f.Path); err != nil {
				return "", err
			}
		}
		fw, err := writer.CreateFormFile(f.Field, f.FileName)
		if err != nil {
			return "", err
		}
		if _, err := fw.Write(data); err != nil {
			return "", err
		}
	}
	for k, vs := range form {
		for _, v := range vs {
			if err := writer.WriteField(k, v); err != nil {
				return "", err
			}
		}
	}
	return writer.FormDataContentType(), writer.Close()
}

// AssertStatus 校验状态码 2xx
func (resp *Response) AssertStatus() error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("url: %s, status: %d", resp.Request.URL, resp.StatusCode)
	}
	return nil
}

// AssertXml 校验响应为 XML 内容
func (resp *Response) AssertXml() error {
	u := resp.Request.URL.String()
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		return fmt.Errorf("ContentType 为空, %s", u)
	}
	isXML := strings.HasPrefix(ct, "application/xml") ||
		strings.HasPrefix(ct, "application/rss+xml") ||
		strings.HasPrefix(ct, "text/xml")
	if !isXML {
		return fmt.Errorf("非 XML 链接, %s %s", u, ct)
	}
	return nil
}
