// Package bencode bencode 解码，用于计算种子 infoHash 与读取种子文件列表
package bencode

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// 跳过一个完整元素，返回元素结束位置（不含）
func skip(b []byte, pos int) (int, error) {
	if pos >= len(b) {
		return 0, errors.New("bencode: 越界")
	}
	switch b[pos] {
	case 'i':
		for i := pos; i < len(b); i++ {
			if b[i] == 'e' {
				return i + 1, nil
			}
		}
		return 0, errors.New("bencode: 整数缺少结束符")
	case 'l', 'd':
		depth := 0
		for i := pos; i < len(b); i++ {
			switch b[i] {
			case 'l', 'd':
				depth++
			case 'e':
				depth--
				if depth == 0 {
					return i + 1, nil
				}
			case 'i':
				j := i + 1
				for ; j < len(b); j++ {
					if b[j] == 'e' {
						break
					}
				}
				if j >= len(b) {
					return 0, errors.New("bencode: 整数缺少结束符")
				}
				i = j
			case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
				colon := -1
				for j := i; j < len(b); j++ {
					if b[j] == ':' {
						colon = j
						break
					}
					if b[j] < '0' || b[j] > '9' {
						break
					}
				}
				if colon < 0 {
					return 0, errors.New("bencode: 字符串缺少冒号")
				}
				n, err := strconv.Atoi(string(b[i:colon]))
				if err != nil {
					return 0, err
				}
				if n > len(b)-colon-1 {
					return 0, errors.New("bencode: 字符串长度越界")
				}
				i = colon + n
			}
		}
		return 0, errors.New("bencode: 容器缺少结束符")
	default:
		if b[pos] < '0' || b[pos] > '9' {
			return 0, errors.New("bencode: 未知类型")
		}
		colon := -1
		for j := pos; j < len(b); j++ {
			if b[j] == ':' {
				colon = j
				break
			}
			if b[j] < '0' || b[j] > '9' {
				break
			}
		}
		if colon < 0 {
			return 0, errors.New("bencode: 字符串缺少冒号")
		}
		n, err := strconv.Atoi(string(b[pos:colon]))
		if err != nil {
			return 0, err
		}
		if n > len(b)-colon-1 {
			return 0, errors.New("bencode: 字符串长度越界")
		}
		return colon + 1 + n, nil
	}
}

// dictGet 在顶层字典中查找 key，返回其值的原始字节区间 [start,end)
func dictGet(b []byte, key string) (int, int, error) {
	if len(b) == 0 || b[0] != 'd' {
		return 0, 0, errors.New("bencode: 顶层不是字典")
	}
	pos := 1
	for pos < len(b) && b[pos] != 'e' {
		kEnd, err := skip(b, pos)
		if err != nil {
			return 0, 0, err
		}
		colon := -1
		for j := pos; j < kEnd; j++ {
			if b[j] == ':' {
				colon = j
				break
			}
		}
		if colon < 0 {
			return 0, 0, errors.New("bencode: 键缺少冒号")
		}
		k := b[colon+1 : kEnd]
		vStart := kEnd
		vEnd, err := skip(b, vStart)
		if err != nil {
			return 0, 0, err
		}
		if string(k) == key {
			return vStart, vEnd, nil
		}
		pos = vEnd
	}
	return 0, 0, errors.New("bencode: 未找到键 " + key)
}

// InfoHash 计算种子的信息哈希，返回 40 位小写十六进制
// 含 v1 元数据（v1 或 hybrid 种子）时为 info 段的 sha1，纯 v2 种子为 info 段 sha256 的前 40 位，与 qBittorrent 的种子 ID 一致
func InfoHash(b []byte) (string, error) {
	start, end, err := dictGet(b, "info")
	if err != nil {
		return "", err
	}
	raw := b[start:end]
	v, err := Decode(raw)
	if err != nil {
		return "", err
	}
	info, isDict := v.(map[string]any)
	if !isDict {
		return "", errors.New("bencode: info 不是字典")
	}
	if _, hasV1 := info["pieces"]; !hasV1 && info["meta version"] == int64(2) {
		sum := sha256.Sum256(raw)
		return hex.EncodeToString(sum[:])[:40], nil
	}
	sum := sha1.Sum(raw)
	return hex.EncodeToString(sum[:]), nil
}

// maxDepth 解码时允许的最大嵌套深度
const maxDepth = 64

// Decode 解码 bencode 数据，结果类型为 map[string]any、[]any、int64 或 string
func Decode(b []byte) (any, error) {
	v, end, err := decodeAt(b, 0, 0)
	if err != nil {
		return nil, err
	}
	if end != len(b) {
		return nil, errors.New("bencode: 存在多余数据")
	}
	return v, nil
}

// decodeAt 解码 pos 处的元素，返回值与结束位置（不含）
func decodeAt(b []byte, pos, depth int) (any, int, error) {
	if depth > maxDepth {
		return nil, 0, errors.New("bencode: 嵌套过深")
	}
	if pos >= len(b) {
		return nil, 0, errors.New("bencode: 越界")
	}
	switch b[pos] {
	case 'i':
		end, err := skip(b, pos)
		if err != nil {
			return nil, 0, err
		}
		n, err := strconv.ParseInt(string(b[pos+1:end-1]), 10, 64)
		if err != nil {
			return nil, 0, err
		}
		return n, end, nil
	case 'l':
		list := []any{}
		pos++
		for pos < len(b) && b[pos] != 'e' {
			v, end, err := decodeAt(b, pos, depth+1)
			if err != nil {
				return nil, 0, err
			}
			list = append(list, v)
			pos = end
		}
		if pos >= len(b) {
			return nil, 0, errors.New("bencode: 列表缺少结束符")
		}
		return list, pos + 1, nil
	case 'd':
		dict := map[string]any{}
		pos++
		for pos < len(b) && b[pos] != 'e' {
			k, kEnd, err := decodeAt(b, pos, depth+1)
			if err != nil {
				return nil, 0, err
			}
			key, isStr := k.(string)
			if !isStr {
				return nil, 0, errors.New("bencode: 字典键不是字符串")
			}
			v, vEnd, err := decodeAt(b, kEnd, depth+1)
			if err != nil {
				return nil, 0, err
			}
			dict[key] = v
			pos = vEnd
		}
		if pos >= len(b) {
			return nil, 0, errors.New("bencode: 字典缺少结束符")
		}
		return dict, pos + 1, nil
	default:
		end, err := skip(b, pos)
		if err != nil {
			return nil, 0, err
		}
		colon := pos
		for b[colon] != ':' {
			colon++
		}
		return string(b[colon+1 : end]), end, nil
	}
}

// TorrentFile 种子内的文件
type TorrentFile struct {
	Path   string
	Length int64
}

// Files 获取种子内的文件列表，多文件种子的路径不含根目录名，支持 v2 种子的 file tree
func Files(b []byte) ([]TorrentFile, error) {
	v, err := Decode(b)
	if err != nil {
		return nil, err
	}
	root, isDict := v.(map[string]any)
	if !isDict {
		return nil, errors.New("bencode: 顶层不是字典")
	}
	info, isDict := root["info"].(map[string]any)
	if !isDict {
		return nil, errors.New("bencode: 缺少 info")
	}
	name := utf8Field(info, "name")
	files, multi := info["files"].([]any)
	if tree, isV2 := info["file tree"].(map[string]any); !multi && isV2 {
		return v2Files(tree), nil
	}
	if !multi {
		length, _ := info["length"].(int64)
		return []TorrentFile{{Path: name, Length: length}}, nil
	}
	out := make([]TorrentFile, 0, len(files))
	for _, f := range files {
		file, isDict := f.(map[string]any)
		if !isDict {
			continue
		}
		parts, _ := file["path.utf-8"].([]any)
		if len(parts) == 0 {
			parts, _ = file["path"].([]any)
		}
		segs := make([]string, 0, len(parts))
		for _, p := range parts {
			if s, isStr := p.(string); isStr {
				segs = append(segs, s)
			}
		}
		length, _ := file["length"].(int64)
		out = append(out, TorrentFile{Path: strings.Join(segs, "/"), Length: length})
	}
	return out, nil
}

// v2Files 展开 v2 种子的 file tree，按键名顺序深度优先遍历，与种子内的文件下标一致
func v2Files(tree map[string]any) []TorrentFile {
	var out []TorrentFile
	// 待处理的目录或文件节点及其路径，两个栈下标一一对应
	dicts := []map[string]any{tree}
	paths := [][]string{nil}
	for len(dicts) > 0 {
		dict, path := dicts[len(dicts)-1], paths[len(paths)-1]
		dicts, paths = dicts[:len(dicts)-1], paths[:len(paths)-1]
		// 文件节点只含空键，其值为文件属性
		if leaf, isFile := dict[""].(map[string]any); isFile {
			length, _ := leaf["length"].(int64)
			out = append(out, TorrentFile{Path: strings.Join(path, "/"), Length: length})
			continue
		}
		keys := slices.Sorted(maps.Keys(dict))
		// 逆序入栈，出栈时按键名顺序处理
		for _, k := range slices.Backward(keys) {
			if child, isDict := dict[k].(map[string]any); isDict {
				dicts = append(dicts, child)
				paths = append(paths, append(slices.Clone(path), k))
			}
		}
	}
	return out
}

func utf8Field(dict map[string]any, key string) string {
	if s, isStr := dict[key+".utf-8"].(string); isStr {
		return s
	}
	s, _ := dict[key].(string)
	return s
}
