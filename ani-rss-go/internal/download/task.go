// 下载器任务操作：查询、删除、重命名与修改保存位置
package download

import (
	"fmt"
	"log/slog"
	"slices"
	"time"

	"ani-rss-go/internal/config"
	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/utils"
)

// GetTorrentsInfos 任务列表
func GetTorrentsInfos() []*entity.TorrentsInfo {
	time.Sleep(time.Second)
	return Get().GetTorrentsInfos()
}

// DeleteTorrent 删除任务，删除本地文件后清理空文件夹
func DeleteTorrent(info *entity.TorrentsInfo, forcedDelete, deleteFiles bool) bool {
	if !forcedDelete && (!config.Get().GetDelete() || !AllowDelete(info)) {
		return false
	}
	slog.Info("删除任务", "name", info.Name, "forcedDelete", forcedDelete, "deleteFiles", deleteFiles)
	time.Sleep(500 * time.Millisecond)
	if ok := Get().Delete(info, deleteFiles); !ok {
		slog.Error("删除任务失败", "name", info.Name)
		return false
	}
	slog.Info("删除任务成功", "name", info.Name)
	if deleteFiles {
		utils.ClearDir(info.SavePath)
	}
	return true
}

// AllowDelete 是否允许删除（等待做种完毕）
func AllowDelete(info *entity.TorrentsInfo) bool {
	if utils.Deref(config.Get().AwaitStalledUP) {
		return info.State == "stoppedUP"
	}
	return info.Finished()
}

// RenameTorrent 重命名任务，已带 RENAME 标签的任务跳过，成功后添加该标签
func RenameTorrent(info *entity.TorrentsInfo) {
	if !config.Get().GetRename() || slices.Contains(info.TagList, entity.TagRename) {
		return
	}
	time.Sleep(time.Second)
	if Get().Rename(info) {
		Get().AddTags(info, entity.TagRename)
	}
}

// SetSavePath 修改任务保存位置，并等待下载器移动文件
func SetSavePath(info *entity.TorrentsInfo, path string) {
	slog.Info(fmt.Sprintf("修改保存位置 %s ==> %s", info.Name, path))
	Get().SetSavePath(info, path)
	time.Sleep(3 * time.Second)
}

// FindTorrentsInfosByAni 根据订阅查询任务列表
func FindTorrentsInfosByAni(anis ...*entity.Ani) []*entity.TorrentsInfo {
	infos := GetTorrentsInfos()
	var out []*entity.TorrentsInfo
	for _, ani := range anis {
		path := GetDownloadPath(ani)
		for _, info := range infos {
			if info.SavePath == path {
				out = append(out, info)
			}
		}
	}
	return out
}
