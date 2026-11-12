package download

import (
	"testing"

	"ani-rss-go/internal/entity"
)

func TestProgressUsesPercent(t *testing.T) {
	for _, tt := range []struct {
		name      string
		completed int64
		size      int64
		progress  float64
		finished  bool
	}{
		{name: "half", completed: 512, size: 1024, progress: 50},
		{name: "complete", completed: 1024, size: 1024, progress: 100, finished: true},
		{name: "empty", progress: 0},
		{name: "rounding", completed: 1, size: 3, progress: 33.33},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := &entity.TorrentsInfo{State: "stoppedUP", Progress: 0.5}
			setProgress(info, tt.completed, tt.size)
			if info.Progress != tt.progress || info.Finished() != tt.finished {
				t.Fatalf("进度或完成状态错误: %+v", info)
			}
			if info.FormatSize == "" {
				t.Fatal("缺少格式化大小")
			}
		})
	}
}

func TestGetFileReName(t *testing.T) {
	for _, tt := range []struct{ name, want string }{
		{name: "release.mkv", want: "番剧 S01E01.mkv"},
		{name: "release.chs.ass", want: "番剧 S01E01.chs.ass"},
		{name: "poster.jpg", want: "poster.jpg"},
	} {
		if got := GetFileReName(tt.name, "番剧 S01E01"); got != tt.want {
			t.Errorf("文件 %q: 得到 %q，期望 %q", tt.name, got, tt.want)
		}
	}
}
