package notification

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
)

func TestTemplateExpandsGlobalOnce(t *testing.T) {
	cfg := config.Get()
	oldTemplate := cfg.NotificationTemplate
	t.Cleanup(func() { cfg.NotificationTemplate = oldTemplate })
	cfg.NotificationTemplate = "${notification} ${title} ${action} ${episode}"
	ani := &entity.Ani{Title: "番剧"}
	n := &entity.NotificationConfig{NotificationTemplate: "${notification}"}
	got := replaceTemplate(ani, n, "S01E02 下载完成", entity.StatusDownloadEnd)
	if got != "番剧 下载完成 2" || strings.Contains(got, "${") {
		t.Fatalf("全局模板展开错误: %q", got)
	}
}

func TestFileMoveTestPreservesFiles(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	name := "番剧 S01E01.mkv"
	if err := os.WriteFile(filepath.Join(src, name), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	n := &entity.NotificationConfig{
		NotificationType: entity.NotifyTypeFileMove,
		StatusList:       []string{"DOWNLOAD_END"},
		FileMoveTarget:   dst,
	}
	if !SendTest(n, &entity.Ani{}, "测试", entity.StatusDownloadEnd) {
		t.Fatal("文件移动配置校验失败")
	}
	if _, err := os.Stat(filepath.Join(src, name)); err != nil {
		t.Fatalf("测试通知修改了源文件: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, name)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("测试通知写入了目标文件: %v", err)
	}
}

func TestOpenListTestOnlyChecksAccount(t *testing.T) {
	var accountCalls, writeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/me" && r.Method == http.MethodGet {
			accountCalls.Add(1)
		} else {
			writeCalls.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"content":[]}}`))
	}))
	t.Cleanup(server.Close)
	n := &entity.NotificationConfig{
		NotificationType:     entity.NotifyTypeOpenListUpload,
		StatusList:           []string{"DOWNLOAD_END"},
		OpenListUploadHost:   server.URL,
		OpenListUploadApiKey: "test-key",
	}
	if !SendTest(n, &entity.Ani{}, "测试", entity.StatusDownloadEnd) {
		t.Fatal("OpenList 账户校验失败")
	}
	if accountCalls.Load() != 1 || writeCalls.Load() != 0 {
		t.Fatalf("测试请求不符合只读约束: account=%d writes=%d", accountCalls.Load(), writeCalls.Load())
	}
}

func TestBarkUsesBatchPush(t *testing.T) {
	var body map[string]any
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	n := &entity.NotificationConfig{
		BarkServerUrl:        server.URL,
		BarkDeviceKeys:       []string{"one", "two"},
		NotificationTemplate: "${title}: ${text}",
	}
	if !barkSend(n, &entity.Ani{Title: "番剧"}, "完成", entity.StatusDownloadEnd) {
		t.Fatal("Bark 请求失败")
	}
	keys, ok := body["device_keys"].([]any)
	if method != http.MethodPost || path != "/push" || !ok || len(keys) != 2 || body["body"] != "番剧: 完成" {
		t.Fatalf("Bark 请求格式错误: %s %s %+v", method, path, body)
	}
}

// fakeSMTP 启动只接受一次连接的最简 SMTP 服务器，dataReply 为邮件内容结束后的答复
func fakeSMTP(t *testing.T, dataReply string) (string, <-chan string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	received := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tc := textproto.NewConn(conn)
		tc.PrintfLine("220 localhost ESMTP")
		for {
			line, err := tc.ReadLine()
			if err != nil {
				return
			}
			switch cmd, _, _ := strings.Cut(line, " "); strings.ToUpper(cmd) {
			case "EHLO":
				tc.PrintfLine("250-localhost")
				tc.PrintfLine("250 AUTH PLAIN")
			case "AUTH":
				tc.PrintfLine("235 ok")
			case "MAIL", "RCPT":
				tc.PrintfLine("250 ok")
			case "DATA":
				tc.PrintfLine("354 go ahead")
				body, err := tc.ReadDotBytes()
				if err != nil {
					return
				}
				received <- string(body)
				tc.PrintfLine("%s", dataReply)
			case "QUIT":
				tc.PrintfLine("221 bye")
				return
			default:
				tc.PrintfLine("502 unknown")
			}
		}
	}()
	return ln.Addr().String(), received
}

func TestSMTPSendTimesOutOnSilentServer(t *testing.T) {
	oldDial, oldSession := smtpDialTimeout, smtpSessionTimeout
	t.Cleanup(func() { smtpDialTimeout, smtpSessionTimeout = oldDial, oldSession })
	smtpDialTimeout, smtpSessionTimeout = 300*time.Millisecond, 300*time.Millisecond

	for _, useSSL := range []bool{true, false} {
		// 服务器接受连接但从不回应
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })
		go func() {
			if conn, err := ln.Accept(); err == nil {
				t.Cleanup(func() { conn.Close() })
			}
		}()
		done := make(chan error, 1)
		go func() {
			done <- smtpSend(ln.Addr().String(), "127.0.0.1", useSSL, "a@b.c", "pw", []string{"d@e.f"}, []byte("x"))
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("ssl=%v: 无响应的服务器不应发送成功", useSSL)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("ssl=%v: 服务器无响应时 smtpSend 没有超时", useSSL)
		}
	}
}

func TestSMTPSendReportsRejectedMail(t *testing.T) {
	addr, received := fakeSMTP(t, "550 rejected as spam")
	err := smtpSend(addr, "127.0.0.1", false, "a@b.c", "pw", []string{"d@e.f"}, []byte("hello"))
	if err == nil || !strings.Contains(err.Error(), "550") {
		t.Fatalf("服务器拒收应返回错误，实际: %v", err)
	}
	if body := <-received; body != "hello\n" {
		t.Fatalf("邮件内容错误: %q", body)
	}
}

func TestSMTPSendSuccess(t *testing.T) {
	addr, received := fakeSMTP(t, "250 queued")
	if err := smtpSend(addr, "127.0.0.1", false, "a@b.c", "pw", []string{"d@e.f", "g@h.i"}, []byte("hello")); err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if body := <-received; body != "hello\n" {
		t.Fatalf("邮件内容错误: %q", body)
	}
}
