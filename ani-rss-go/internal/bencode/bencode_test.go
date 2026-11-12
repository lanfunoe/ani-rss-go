package bencode

import (
	"crypto/sha1"
	"fmt"
	"testing"
)

// TestInfoHashWithInteger 字典与列表内部的整数元素应正确跳过
func TestInfoHashWithInteger(t *testing.T) {
	if _, err := InfoHash([]byte("d4:infod6:lengthi5eee")); err != nil {
		t.Fatalf("解析包含整数的字典失败: %v", err)
	}
	if _, err := InfoHash([]byte("d4:infod5:filesl6:lengthi5eeee")); err != nil {
		t.Fatalf("解析包含整数的列表失败: %v", err)
	}
	if _, err := InfoHash([]byte("d4:infod6:lengthi5e")); err == nil {
		t.Fatal("整数缺少结束符时应返回错误")
	}
}

// TestInfoHashRawRegion InfoHash 必须是对 info 字典原始字节的 sha1
func TestInfoHashRawRegion(t *testing.T) {
	info := []byte("d6:lengthi5e4:name3:abce")
	torrent := append(append([]byte("d4:info"), info...), 'e')
	want := fmt.Sprintf("%x", sha1.Sum(info))
	got, err := InfoHash(torrent)
	if err != nil {
		t.Fatalf("InfoHash 失败: %v", err)
	}
	if got != want {
		t.Fatalf("InfoHash = %s, 期望 %s", got, want)
	}
}

// TestFiles 多文件种子返回不含根目录的路径，单文件种子返回 name
func TestFiles(t *testing.T) {
	multi := []byte("d4:infod5:filesld6:lengthi10e4:pathl3:sub5:a.mkveed6:lengthi20e4:pathl5:b.asseee4:name4:rootee")
	files, err := Files(multi)
	if err != nil {
		t.Fatalf("解析多文件种子失败: %v", err)
	}
	if len(files) != 2 || files[0].Path != "sub/a.mkv" || files[0].Length != 10 || files[1].Path != "b.ass" {
		t.Fatalf("多文件种子解析结果错误: %+v", files)
	}

	single := []byte("d4:infod6:lengthi7e4:name5:x.mkvee")
	files, err = Files(single)
	if err != nil || len(files) != 1 || files[0].Path != "x.mkv" || files[0].Length != 7 {
		t.Fatalf("单文件种子解析结果错误: %+v, %v", files, err)
	}

	if _, err := Files([]byte("d4:infod5:filesl")); err == nil {
		t.Fatal("截断的种子应返回错误")
	}
}
