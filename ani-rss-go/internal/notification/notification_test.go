package notification

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

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
