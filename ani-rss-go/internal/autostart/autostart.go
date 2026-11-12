// Package autostart 开机自启动：仅支持 Linux 运行，暂不支持开机自启动
package autostart

// Sync 按开关启用或禁用开机自启动
func Sync(enabled bool) error {
	// Linux 下不支持开机自启动，忽略
	_ = enabled
	return nil
}
