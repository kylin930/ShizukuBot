// Package selfupdate 用新构建的二进制替换正在运行的主程序并重新启动。
//
// Windows 无法覆盖正在运行的 exe，因此构建产物先写到主程序旁的 .new 文件；
// 主程序退出后，由独立的后台脚本完成替换和重启，期间无需手动停止服务。
package selfupdate

import "os"

// Staged 返回构建产物路径：与主程序同目录，避免跨盘移动。
func Staged(executable string) string { return executable + ".new" }

// Exists 判断构建产物是否可用于重启。
func Exists(staged string) bool {
	info, err := os.Stat(staged)
	return err == nil && info.Size() > 0
}
