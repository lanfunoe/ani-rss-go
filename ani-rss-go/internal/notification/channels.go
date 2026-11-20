// 通知渠道实现：Telegram / Bark / AstrBot / Server酱 / WebHook / 邮件 / Shell / 其他
package notification

import (
	"bytes"
	"cmp"
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/download"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/httpc"
	"ani-rss-go/internal/store"
	"ani-rss-go/internal/utils"
)

// telegramSend Telegram 通知
func telegramSend(n *entity.NotificationConfig, ani *entity.Ani, text string, status entity.NotificationStatus) bool {
	token, chatId := n.TelegramBotToken, n.TelegramChatId
	if chatId == "" || token == "" {
		slog.Warn("telegram 通知的参数不完整")
		return false
	}
	host := cmp.Or(n.TelegramApiHost, "https://api.telegram.org")
	content := replaceTemplate(ani, n, text, status)

	photo := ""
	if utils.Deref(n.TelegramImage) && ani != nil && ani.Cover != "" {
		photo = store.CoverFile(ani.Cover)
	}
	if photo != "" {
		req := httpc.Post(host+"/bot"+token+"/sendPhoto").
			SetForm("chat_id", chatId).
			SetForm("caption", content).
			AddFile("photo", photo)
		if n.TelegramFormat != "" {
			req.SetForm("parse_mode", n.TelegramFormat)
		}
		if n.TelegramTopicId > -1 {
			req.SetForm("message_thread_id", strconv.Itoa(n.TelegramTopicId))
		}
		resp, err := req.Do()
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode < 300
	}

	body := map[string]any{
		"chat_id": chatId,
		"text":    content,
	}
	if n.TelegramTopicId > -1 {
		body["message_thread_id"] = n.TelegramTopicId
	}
	if n.TelegramFormat != "" {
		body["parse_mode"] = n.TelegramFormat
	}
	return postJSON(fmt.Sprintf("%s/bot%s/sendMessage", host, token), body)
}

// barkSend Bark 通知（单次 POST /push）
func barkSend(n *entity.NotificationConfig, ani *entity.Ani, text string, status entity.NotificationStatus) bool {
	if len(n.BarkDeviceKeys) == 0 {
		slog.Warn("请设置 Bark DeviceKeys")
		return false
	}
	if strings.TrimSpace(n.BarkServerUrl) == "" {
		slog.Warn("请设置 Bark ServerUrl")
		return false
	}
	content := replaceTemplate(ani, n, text, status)

	image := ""
	if ani != nil {
		image = ani.Image
	}
	body := map[string]any{
		"device_keys": n.BarkDeviceKeys,
		"title":       "ani-rss",
		"subtitle":    text,
		"icon":        "https://docs.wushuo.top/favicon.ico",
		"image":       image,
		"group":       n.BarkGroup,
		"level":       n.BarkLevel,
		"volume":      n.BarkVolume,
	}
	if utils.Deref(n.BarkUseMarkdown) {
		body["markdown"] = content
	} else {
		body["body"] = content
	}
	return postJSON(strings.TrimRight(n.BarkServerUrl, "/")+"/push", body)
}

// astrBotSend 通过 AstrBot OpenAPI 向指定会话推送通知，开启发送图片且本地封面存在时附带封面
func astrBotSend(n *entity.NotificationConfig, ani *entity.Ani, text string, status entity.NotificationStatus) bool {
	host := strings.TrimRight(strings.TrimSpace(n.AstrBotHost), "/")
	apiKey := strings.TrimSpace(n.AstrBotApiKey)
	umo := strings.TrimSpace(n.AstrBotUmo)
	switch {
	case host == "":
		slog.Warn("请设置 AstrBot 服务地址")
		return false
	case apiKey == "":
		slog.Warn("请设置 AstrBot API Key（需要 im 权限）")
		return false
	case umo == "":
		slog.Warn("请设置 AstrBot 目标 UMO（在目标会话发送 /sid 获取）")
		return false
	}
	message := replaceTemplate(ani, n, text, status)
	if message == "" {
		slog.Warn("AstrBot 通知内容不能为空，请检查通知模版")
		return false
	}

	// 关闭图片或封面缺失时只发送文本，兼容只具有 im 权限的密钥
	var content any = message
	if utils.DerefOr(n.AstrBotImage, true) && ani != nil {
		if photo := store.CoverFile(ani.Cover); photo != "" {
			// 先上传封面获取附件 ID，再将图片与正文放在同一次消息请求中发送，上传需要 file 权限
			resp, err := httpc.Post(host+"/api/v1/file").
				Set("Authorization", "Bearer "+apiKey).
				AddFile("file", photo).
				Do()
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			if err := resp.AssertStatus(); err != nil {
				slog.Error("AstrBot 上传封面失败", "error", err)
				return false
			}
			var uploaded struct {
				Data struct {
					AttachmentId string `json:"attachment_id"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&uploaded); err != nil || uploaded.Data.AttachmentId == "" {
				slog.Error("AstrBot 上传封面失败", "error", err)
				return false
			}
			content = []map[string]string{
				{"type": "image", "attachment_id": uploaded.Data.AttachmentId},
				{"type": "plain", "text": message},
			}
		}
	}

	body, _ := json.Marshal(map[string]any{"umo": umo, "message": content})
	resp, err := httpc.Post(host+"/api/v1/im/message").
		Set("Authorization", "Bearer "+apiKey).
		Set("Content-Type", "application/json").
		SetBody(body).
		Do()
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	decodeErr := json.NewDecoder(resp.Body).Decode(&result)
	if err := resp.AssertStatus(); err != nil {
		slog.Error("AstrBot 通知发送失败", "error", err, "message", result.Message)
		return false
	}
	if decodeErr != nil || !strings.EqualFold(result.Status, "ok") {
		slog.Error("AstrBot 通知发送失败", "status", result.Status, "message", result.Message)
		return false
	}
	return true
}

// serverChanSend Server酱通知，正文为 Markdown 并附带封面
func serverChanSend(n *entity.NotificationConfig, ani *entity.Ani, text string, status entity.NotificationStatus) bool {
	chan3 := n.ServerChanType == "SERVER_CHAN_3"
	if chan3 && strings.TrimSpace(n.ServerChan3ApiUrl) == "" {
		slog.Warn("apiUrl 不能为空")
		return false
	}
	if !chan3 && strings.TrimSpace(n.ServerChanSendKey) == "" {
		slog.Warn("sendKey 不能为空")
		return false
	}

	title := aniTitle(ani)
	if utils.DerefOr(n.ServerChanTitleAction, true) {
		title = status.Action + "#" + title
	} else if runes := []rune(title); len(runes) > 15 {
		title = string(runes[:12]) + "..."
	}

	image := ""
	if ani != nil {
		image = ani.Image
	}
	content := strings.ReplaceAll(replaceTemplate(ani, n, text, status), "\n", "\n\n")
	body := map[string]any{
		"title": title,
		"desp":  "# " + content + "\n\n![" + image + "](" + image + ")",
	}
	u := "https://sctapi.ftqq.com/" + n.ServerChanSendKey + ".send"
	if chan3 {
		u = n.ServerChan3ApiUrl
		body["tags"] = "ani-rss"
	}
	return postJSON(u, body)
}

// webHookSend WebHook 通知
// 地址与请求体支持通知模版变量；${message} 为 JSON 转义后的全局通知模版，${image} 为封面地址，${imageBase64} 为本地封面的 Base64
func webHookSend(n *entity.NotificationConfig, ani *entity.Ani, text string, status entity.NotificationStatus) bool {
	webHookUrl := expandTemplate(ani, n.WebHookUrl, text, status)
	webHookBody := expandTemplate(ani, n.WebHookBody, text, status)
	if webHookUrl == "" {
		slog.Warn("webhook url is blank")
		return false
	}

	var quoted strings.Builder
	encoder := json.NewEncoder(&quoted)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(expandTemplate(ani, config.Get().NotificationTemplate, text, status))
	message := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(quoted.String(), `"`), "\n"), `"`)

	image := "https://docs.wushuo.top/null.png"
	if ani != nil && ani.Image != "" {
		image = ani.Image
	}
	replacements := []string{"${message}", message, "${image}", image}
	if strings.Contains(webHookBody, "${imageBase64}") {
		// 1*1 像素 gif
		imageBase64 := "R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"
		if ani != nil {
			if data, err := os.ReadFile(store.CoverFile(ani.Cover)); err == nil {
				imageBase64 = base64.StdEncoding.EncodeToString(data)
			}
		}
		replacements = append(replacements, "${imageBase64}", imageBase64)
	}
	replacer := strings.NewReplacer(replacements...)
	webHookUrl = replacer.Replace(webHookUrl)
	webHookBody = replacer.Replace(webHookBody)

	// 地址中的模版变量可能含有空白与换行，按参数重新编码查询串
	if base, query, found := strings.Cut(webHookUrl, "?"); found {
		escape := func(s string) string {
			if unescaped, err := url.QueryUnescape(s); err == nil {
				s = unescaped
			}
			return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
		}
		pairs := strings.Split(query, "&")
		for i, pair := range pairs {
			key, value, hasValue := strings.Cut(pair, "=")
			pairs[i] = escape(key)
			if hasValue {
				pairs[i] += "=" + escape(value)
			}
		}
		webHookUrl = base + "?" + strings.Join(pairs, "&")
	}

	req := httpc.New(strings.ToUpper(cmp.Or(n.WebHookMethod, "POST")), webHookUrl)
	// 自定义 header，每行 key:value
	for line := range strings.SplitSeq(n.WebHookHeader, "\n") {
		key, value, found := strings.Cut(line, ":")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !found || key == "" || value == "" {
			continue
		}
		req.Header[http.CanonicalHeaderKey(key)] = value
	}
	slog.Debug("webhook url: " + webHookUrl)
	if webHookBody != "" {
		slog.Debug("webhook body: " + webHookBody)
		req.Body = []byte(webHookBody)
		// 未自定义 Content-Type 时按请求体内容判断
		if _, exists := req.Header["Content-Type"]; !exists {
			contentType := "application/x-www-form-urlencoded;charset=UTF-8"
			switch webHookBody[0] {
			case '{', '[':
				contentType = "application/json;charset=UTF-8"
			case '<':
				contentType = "application/xml;charset=UTF-8"
			}
			req.Header["Content-Type"] = contentType
		}
	}
	resp, err := req.Do()
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		slog.Error("webhook error", "status", resp.StatusCode)
		return false
	}
	return true
}

//go:embed mail.tmpl
var mailTemplateText string

// mailTemplate 邮件 HTML 模版
var mailTemplate = template.Must(template.New("mail").Parse(mailTemplateText))

type mailData struct {
	Render    template.HTML
	Image     string
	MailImage bool
}

// mailSend 邮件通知
func mailSend(n *entity.NotificationConfig, ani *entity.Ani, text string, status entity.NotificationStatus) bool {
	if n.MailSMTPHost == "" || n.MailFrom == "" || n.MailPassword == "" || n.MailAddressee == "" {
		slog.Warn("邮箱通知的参数不完整")
		return false
	}
	content := replaceTemplate(ani, n, text, status)
	// markdown 渲染前换行加倍
	content = strings.ReplaceAll(content, "\n", "\n\n")

	// 标题过长时使用订阅标题
	subject := text
	if len([]rune(text)) > 200 && ani != nil {
		subject = ani.Title
	}

	var buf strings.Builder
	md := goldmark.New(goldmark.WithRendererOptions(html.WithUnsafe()))
	if err := md.Convert([]byte(content), &buf); err != nil {
		slog.Error("邮件内容渲染失败", "error", err)
		return false
	}

	image := ""
	if ani != nil {
		image = ani.Image
	}
	mailImage := utils.Deref(n.MailImage)
	var htmlBuf strings.Builder
	if err := mailTemplate.Execute(&htmlBuf, mailData{
		Render:    template.HTML(buf.String()),
		Image:     image,
		MailImage: mailImage,
	}); err != nil {
		slog.Error("邮件模版渲染失败", "error", err)
		return false
	}

	addr := fmt.Sprintf("%s:%d", n.MailSMTPHost, n.MailSMTPPort)
	useSSL := utils.DerefOr(n.MailSSLEnable, true)

	// 多个收件人使用 , 或 ; 分隔
	var recipients []string
	for addressee := range strings.FieldsFuncSeq(n.MailAddressee, func(r rune) bool { return r == ',' || r == ';' }) {
		if addressee = strings.TrimSpace(addressee); addressee != "" {
			recipients = append(recipients, addressee)
		}
	}
	msg := buildMail(n.MailFrom, strings.Join(recipients, ", "), subject, htmlBuf.String())
	if err := smtpSend(addr, n.MailSMTPHost, useSSL, n.MailFrom, n.MailPassword, recipients, msg); err != nil {
		slog.Error("邮件发送失败", "error", err)
		return false
	}
	return true
}

var (
	// smtpDialTimeout 建立连接（SSL 时含握手）的超时
	smtpDialTimeout = 15 * time.Second
	// smtpSessionTimeout 整个 SMTP 会话的超时，避免服务器无响应时卡住通知队列
	smtpSessionTimeout = time.Minute
)

// smtpSend 发送邮件，所有网络操作都受超时限制
// useSSL 为 true 时直接建立 SSL 连接，否则建立明文连接并在服务器支持时升级 STARTTLS
func smtpSend(addr, host string, useSSL bool, from, password string, recipients []string, msg []byte) error {
	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	var conn net.Conn
	var err error
	if useSSL {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host})
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("连接 SMTP 失败: %w", err)
	}
	// 截止时间设在底层连接上，STARTTLS 升级后同样生效
	if err = conn.SetDeadline(time.Now().Add(smtpSessionTimeout)); err != nil {
		conn.Close()
		return err
	}
	// NewClient 失败时会自行关闭连接
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("读取 SMTP 欢迎信息失败: %w", err)
	}
	defer c.Close()
	if !useSSL {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err = c.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return fmt.Errorf("STARTTLS 失败: %w", err)
			}
		}
	}
	if err = c.Auth(smtp.PlainAuth("", from, password, host)); err != nil {
		return fmt.Errorf("SMTP 认证失败: %w", err)
	}
	if err = c.Mail(from); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err = c.Rcpt(recipient); err != nil {
			return fmt.Errorf("收件人 %s 被拒绝: %w", recipient, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(msg); err != nil {
		return err
	}
	// Close 会发送结束标记并读取服务器对这封邮件的最终答复，拒收在这里才会体现
	if err = w.Close(); err != nil {
		return err
	}
	// 邮件已被服务器接收，QUIT 失败不算发送失败，避免重试导致重复发信
	if err = c.Quit(); err != nil {
		slog.Debug("SMTP QUIT 失败", "error", err)
	}
	return nil
}

func buildMail(from, to, subject, body string) []byte {
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: ani-rss <%s>\r\n", from)
	fmt.Fprintf(&msg, "To: %s\r\n", to)
	fmt.Fprintf(&msg, "Subject: =?UTF-8?B?%s?=\r\n", base64.StdEncoding.EncodeToString([]byte(subject)))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	msg.WriteString(base64.StdEncoding.EncodeToString([]byte(body)))
	return msg.Bytes()
}

// shellSend Shell 通知，命令支持通知模版变量，超过存活限制时强制停止
func shellSend(n *entity.NotificationConfig, ani *entity.Ani, text string, status entity.NotificationStatus) bool {
	if strings.TrimSpace(n.Shell) == "" {
		slog.Warn("shell 不能为空")
		return false
	}
	shellConfig := *n
	shellConfig.NotificationTemplate = n.Shell
	script := replaceTemplate(ani, &shellConfig, text, status)
	slog.Debug(script)

	ctx := context.Background()
	if n.AliveLimit > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(n.AliveLimit)*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	// 命令启动的后台进程仍占用输出时，最多再等待 5 秒
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		slog.Error(err.Error())
		return false
	}
	pid := cmd.Process.Pid
	slog.Info(fmt.Sprintf("pid: %d", pid))
	err := cmd.Wait()
	slog.Debug(output.String())
	if ctx.Err() != nil {
		slog.Info(fmt.Sprintf("存活超时已强制停止 pid: %d", pid))
		return false
	}
	slog.Info(fmt.Sprintf("已退出 pid: %d, exit: %d", pid, cmd.ProcessState.ExitCode()))
	return err == nil
}

// systemSend 系统通知，未实现系统托盘，写入日志
func systemSend(n *entity.NotificationConfig, ani *entity.Ani, text string, status entity.NotificationStatus) bool {
	slog.Info("系统通知", "text", replaceTemplate(ani, n, text, status))
	return true
}

// embyRefreshSend 延迟后扫描所选且仍存在的 Emby 媒体库
func embyRefreshSend(n *entity.NotificationConfig, _ *entity.Ani, _ string, _ entity.NotificationStatus) bool {
	if n.EmbyDelayed > 0 {
		time.Sleep(time.Duration(n.EmbyDelayed) * time.Second)
	}
	views, err := EmbyViewList(n)
	if err != nil {
		slog.Error(err.Error())
		return false
	}
	query := "Recursive=true&ImageRefreshMode=Default&MetadataRefreshMode=Default&ReplaceAllImages=false&ReplaceAllMetadata=false"
	for _, view := range views {
		if !slices.Contains(n.EmbyRefreshViewIds, view.ID) {
			continue
		}
		resp, err := httpc.Post(strings.TrimRight(n.EmbyHost, "/")+"/emby/Items/"+view.ID+"/Refresh?"+query).
			Set("X-Emby-Token", n.EmbyApiKey).
			Do()
		if err != nil {
			return false
		}
		resp.Body.Close()
		if err := resp.AssertStatus(); err != nil {
			slog.Error("扫描 Emby 媒体库出错", "id", view.ID, "name", view.Name, "status", resp.StatusCode)
			continue
		}
		slog.Info("正在扫描 Emby 媒体库", "id", view.ID, "name", view.Name)
	}
	return true
}

// fileMoveSend 文件移动通知
func fileMoveSend(n *entity.NotificationConfig, ani *entity.Ani, _ string, status entity.NotificationStatus) bool {
	if status != entity.StatusDownloadEnd {
		slog.Info("文件移动 仅支持下载完成通知")
		return true
	}
	if ani == nil {
		return true
	}
	// 首先克隆 防止影响原订阅设置
	a := *ani

	// 旧的位置
	src := download.GetDownloadPath(&a)

	// 新的位置; 设置自定义下载位置同时启用, 用以获取新的位置
	template := n.FileMoveTarget
	if a.GetOva() {
		template = n.FileMoveOvaTarget
	}
	target := download.GetDownloadPathByTemplate(&a, template)

	// 进行移动
	if err := os.MkdirAll(target, 0o755); err != nil {
		slog.Error("创建目标文件夹失败", "target", target, "error", err)
		return false
	}

	if a.GetOva() {
		startMoveOva(n, src, target)
		return true
	}

	if utils.Deref(n.FileMoveDeleteOldEpisode) {
		deleteOldEpisodeFiles(src, target)
	}

	startMove(n, src, target)
	return true
}

// startMoveOva OVA/剧场版 移动全部文件
func startMoveOva(n *entity.NotificationConfig, src, target string) {
	copyModel := utils.Deref(n.FileMoveCopyModel)
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		full := filepath.Join(src, entry.Name())
		slog.Info("OVA/剧场版 文件移动", "from", full, "to", target)
		if err := utils.CopyFile(full, filepath.Join(target, entry.Name())); err != nil {
			slog.Error("文件移动失败", "from", full, "error", err)
			continue
		}
		if copyModel {
			continue
		}
		// 复制后再删除 确保不会中途失败
		os.Remove(full)
	}
}

// startMove 移动视频与字幕文件
func startMove(n *entity.NotificationConfig, src, target string) {
	copyModel := utils.Deref(n.FileMoveCopyModel)

	// 目标位置文件列表 存储为MAP便于根据文件名寻找
	targetFileMap := map[string]int64{}
	entries, err := os.ReadDir(target)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if st, e := entry.Info(); e == nil {
				targetFileMap[entry.Name()] = st.Size()
			}
		}
	}

	srcEntries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, entry := range srcEntries {
		if entry.IsDir() {
			// 过滤掉文件夹
			continue
		}
		name := entry.Name()
		if !isVideoOrSubtitleName(name) {
			// 非视频与字幕文件
			continue
		}
		if entity.SeasonReg.FindString(name) == "" {
			// 确保命名
			continue
		}

		full := filepath.Join(src, name)
		st, e := entry.Info()
		if e != nil {
			continue
		}
		if size, ok := targetFileMap[name]; ok && size == st.Size() {
			// 文件名与大小一致 跳过移动
			continue
		}

		slog.Info("文件移动", "from", full, "to", target)
		if err := utils.CopyFile(full, filepath.Join(target, name)); err != nil {
			slog.Error("文件移动失败", "from", full, "error", err)
			continue
		}
		if copyModel {
			continue
		}
		// 复制后再删除 确保不会中途失败
		os.Remove(full)
	}
}

// deleteOldEpisodeFiles 删除目标位置的同集视频, 以实现洗版效果
func deleteOldEpisodeFiles(src, target string) {
	// 原位置文件列表
	srcFileMap := map[string]int64{}
	srcEntries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	episodeSet := map[string]bool{}
	for _, entry := range srcEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		size := int64(-1)
		if st, e := entry.Info(); e == nil {
			size = st.Size()
		}
		srcFileMap[name] = size
		tag := entity.SeasonReg.FindString(name)
		if isVideoOrSubtitleName(name) && tag != "" {
			episodeSet[strings.ToUpper(tag)] = true
		}
	}

	files, err := os.ReadDir(target)
	if err != nil {
		return
	}
	for _, entry := range files {
		name := entry.Name()
		if !isVideoOrSubtitleName(name) {
			// 非视频与字幕文件
			continue
		}
		if entity.SeasonReg.FindString(name) == "" {
			// 确保命名
			continue
		}

		if srcSize, ok := srcFileMap[name]; ok {
			size := int64(-1)
			if st, e := entry.Info(); e == nil {
				size = st.Size()
			}
			if size == srcSize {
				// 文件名与大小一致 跳过删除
				continue
			}
		}

		episode := strings.ToUpper(entity.SeasonReg.FindString(name))
		if !episodeSet[episode] {
			// 新位置没有旧的同集文件, 不需要删除
			continue
		}

		full := filepath.Join(target, name)
		slog.Info("因洗版需要删除", "file", full)

		// 删除 洗版
		os.Remove(full)
	}
}

func isVideoOrSubtitleName(name string) bool {
	return utils.IsVideoFormat(name) || utils.IsSubtitleFormat(name)
}

// openListFileInfo OpenList 文件信息
type openListFileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// openListUploadSend OpenList 上传通知
func openListUploadSend(n *entity.NotificationConfig, ani *entity.Ani, _ string, status entity.NotificationStatus) bool {
	if status != entity.StatusDownloadEnd {
		slog.Info("OpenListUpload 仅支持下载完成通知")
		return true
	}
	if ani == nil {
		return true
	}
	// 首先克隆 防止影响原订阅设置
	a := *ani

	// 本地位置
	localPath := download.GetDownloadPath(&a)

	// 新的位置; 设置自定义下载位置同时启用, 用以获取新的位置
	template := n.OpenListUploadPath
	if a.GetOva() {
		template = n.OpenListUploadOvaPath
	}
	if utils.Deref(a.CustomUploadEnable) {
		// 自定义上传位置
		template = a.CustomUploadPathTarget
	}

	target := download.GetDownloadPathByTemplate(&a, template)
	target = entity.WinDriveReg.ReplaceAllString(target, "")

	if a.GetOva() {
		return openListUploadOva(n, localPath, target)
	}

	if utils.Deref(n.OpenListUploadDeleteOldEpisode) {
		if !openListDeleteOldEpisode(n, localPath, target) {
			return false
		}
	}

	return openListUploadDir(n, localPath, target)
}

// openListUploadOva 上传全部视频与字幕
func openListUploadOva(n *entity.NotificationConfig, localPath, cloudPath string) bool {
	entries, err := os.ReadDir(localPath)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			// 文件夹 跳过
			continue
		}
		name := entry.Name()
		if !isVideoOrSubtitleName(name) {
			// 非视频与字幕 跳过
			continue
		}

		// 开始上传
		if !openListUploadFile(n, filepath.Join(localPath, name), cloudPath) {
			return false
		}
	}
	return true
}

// openListDeleteOldEpisode 删除云端同集文件, 以实现洗版效果
func openListDeleteOldEpisode(n *entity.NotificationConfig, localPath, cloudPath string) bool {
	localFileMap := map[string]int64{}
	localEpisodeSet := map[string]bool{}
	entries, err := os.ReadDir(localPath)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		size := int64(-1)
		if st, e := entry.Info(); e == nil {
			size = st.Size()
		}
		localFileMap[name] = size
		tag := entity.SeasonReg.FindString(name)
		if isVideoOrSubtitleName(name) && tag != "" {
			localEpisodeSet[strings.ToUpper(tag)] = true
		}
	}

	fileInfos := openListFsList(n, cloudPath)
	for _, fileInfo := range fileInfos {
		name := fileInfo.Name
		if !isVideoOrSubtitleName(name) {
			// 非视频与字幕文件
			continue
		}
		if entity.SeasonReg.FindString(name) == "" {
			// 确保命名
			continue
		}

		if size, ok := localFileMap[name]; ok && fileInfo.Size == size {
			// 文件名与大小一致 跳过删除
			continue
		}

		episode := strings.ToUpper(entity.SeasonReg.FindString(name))
		if !localEpisodeSet[episode] {
			// 新位置没有旧的同集文件 不需要删除
			continue
		}

		slog.Info("因洗版需要删除", "file", name)

		// 删除 洗版
		if !openListFsRemove(n, cloudPath, []string{name}) {
			return false
		}
	}
	return true
}

// openListUploadDir 上传视频与字幕文件
func openListUploadDir(n *entity.NotificationConfig, localPath, cloudPath string) bool {
	deleteLocalFile := utils.Deref(n.OpenListUploadDeleteLocalFile)

	// 云端文件列表 存储为MAP便于根据文件名寻找
	cloudFileMap := map[string]int64{}
	for _, fileInfo := range openListFsList(n, cloudPath) {
		cloudFileMap[fileInfo.Name] = fileInfo.Size
	}

	entries, err := os.ReadDir(localPath)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			// 过滤掉文件夹
			continue
		}
		name := entry.Name()
		if !isVideoOrSubtitleName(name) {
			// 非视频与字幕文件
			continue
		}
		if entity.SeasonReg.FindString(name) == "" {
			// 确保命名
			continue
		}

		full := filepath.Join(localPath, name)
		size := int64(-1)
		if st, e := entry.Info(); e == nil {
			size = st.Size()
		}
		if cloudSize, ok := cloudFileMap[name]; ok && size == cloudSize {
			// 文件名与大小一致 跳过上传
			continue
		}

		slog.Info("文件上传", "from", full, "to", cloudPath)

		if !openListUploadFile(n, full, cloudPath) {
			return false
		}

		if deleteLocalFile {
			slog.Info("删除本地文件", "file", full)
			os.Remove(full)
		}
	}
	return true
}

// openListUploadFile 流式上传单个文件，不限制上传总时长
func openListUploadFile(n *entity.NotificationConfig, localFilePath, cloudFilePath string) bool {
	file, err := os.Open(localFilePath)
	if err != nil {
		slog.Error("读取文件失败", "file", localFilePath, "error", err)
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		slog.Error("读取文件失败", "file", localFilePath, "error", err)
		return false
	}
	filename := filepath.Base(localFilePath)
	resp, err := httpc.Put(n.OpenListUploadHost+"/api/fs/put").
		SetTimeout(-1).
		Set("Authorization", n.OpenListUploadApiKey).
		Set("As-Task", "false").
		Set("File-Path", url.PathEscape(cloudFilePath+"/"+filename)).
		Set("Content-Type", "application/octet-stream").
		SetBodyReader(file, info.Size()).
		Do()
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		slog.Error("上传失败", "file", localFilePath, "status", resp.StatusCode)
		return false
	}
	out := struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false
	}
	if out.Code != 200 {
		slog.Error("上传失败", "file", localFilePath, "code", out.Code, "message", out.Message)
		return false
	}
	slog.Info("OpenList 上传完成", "file", filename)
	return true
}

// openListFsList 获取云端文件列表
func openListFsList(n *entity.NotificationConfig, path string) []openListFileInfo {
	// 间隔两秒，避免频繁请求网盘触发流控
	time.Sleep(2 * time.Second)
	body, _ := json.Marshal(map[string]any{
		"path":     path,
		"page":     1,
		"per_page": 0,
		"refresh":  true,
	})
	resp, err := httpc.Post(n.OpenListUploadHost+"/api/fs/list").
		Set("Authorization", n.OpenListUploadApiKey).
		Set("Content-Type", "application/json").
		SetBody(body).
		Do()
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	out := struct {
		Code int `json:"code"`
		Data struct {
			Content []openListFileInfo `json:"content"`
		} `json:"data"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Code != 200 {
		return nil
	}
	return out.Data.Content
}

// openListFsRemove 删除云端文件
func openListFsRemove(n *entity.NotificationConfig, dir string, names []string) bool {
	// 间隔两秒，避免频繁请求网盘触发流控
	time.Sleep(2 * time.Second)
	body, _ := json.Marshal(map[string]any{
		"dir":   dir,
		"names": names,
	})
	resp, err := httpc.Post(n.OpenListUploadHost+"/api/fs/remove").
		Set("Authorization", n.OpenListUploadApiKey).
		Set("Content-Type", "application/json").
		SetBody(body).
		Do()
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 300
}

// TgUpdates 获取 TG 最近消息
// 会话 id 以字符串返回，前端直接作为 telegramChatId 保存
func TgUpdates(n *entity.NotificationConfig) ([]map[string]any, error) {
	chats := []map[string]any{}
	if n.TelegramBotToken == "" {
		return chats, nil
	}
	host := cmp.Or(n.TelegramApiHost, "https://api.telegram.org")
	resp, err := httpc.Get(fmt.Sprintf("%s/bot%s/getUpdates", host, n.TelegramBotToken)).Do()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	type message struct {
		Chat *struct {
			ID        int64  `json:"id"`
			Type      string `json:"type"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		} `json:"chat"`
	}
	out := struct {
		Result []struct {
			Message      *message `json:"message"`
			MyChatMember *message `json:"my_chat_member"`
		} `json:"result"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	for _, r := range out.Result {
		// 机器人被加入群组或频道时只有 my_chat_member
		m := cmp.Or(r.Message, r.MyChatMember)
		if m == nil || m.Chat == nil {
			continue
		}
		id := strconv.FormatInt(m.Chat.ID, 10)
		if slices.ContainsFunc(chats, func(c map[string]any) bool { return c["id"] == id }) {
			continue
		}
		username := m.Chat.Username
		if username == "" {
			username = strings.TrimSpace(m.Chat.FirstName + " " + m.Chat.LastName)
		}
		chats = append(chats, map[string]any{
			"id":        id,
			"firstName": m.Chat.FirstName,
			"lastName":  m.Chat.LastName,
			"username":  username,
			"type":      m.Chat.Type,
		})
	}
	return chats, nil
}

// EmbyViewList 获取 Emby 媒体库列表
func EmbyViewList(n *entity.NotificationConfig) ([]entity.EmbyView, error) {
	if n.EmbyHost == "" {
		return nil, errors.New("embyHost 为空")
	}
	if n.EmbyApiKey == "" {
		return nil, errors.New("embyApiKey 为空")
	}
	resp, err := httpc.Get(strings.TrimRight(n.EmbyHost, "/")+"/Library/MediaFolders").
		Set("X-Emby-Token", n.EmbyApiKey).
		Do()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return nil, fmt.Errorf("embyHost 响应异常: %d", resp.StatusCode)
	}
	out := struct {
		Items []struct {
			ID   string `json:"Id"`
			Name string `json:"Name"`
		} `json:"Items"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	views := make([]entity.EmbyView, 0, len(out.Items))
	for _, item := range out.Items {
		views = append(views, entity.EmbyView{ID: item.ID, Name: item.Name})
	}
	return views, nil
}

func postJSON(u string, body any) bool {
	data, _ := json.Marshal(body)
	resp, err := httpc.Post(u).
		Set("Content-Type", "application/json").
		SetBody(data).
		Do()
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 300
}

func aniTitle(a *entity.Ani) string {
	if a == nil {
		return ""
	}
	return a.Title
}
