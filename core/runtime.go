package core

import (
	"fmt"
	"runtime"
	"time"
)

// StartTime 程序启动时间（包初始化时记录，与 main 入口几乎同时）
var StartTime = time.Now()

// CoreUptime 返回自启动以来的运行时长，格式 "1d 2h 3m"（不足的天/时省略）
func CoreUptime() string {
	d := time.Since(StartTime)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// CoreOS 返回运行系统信息，格式 "goos/goarch"，如 "windows/amd64"
func CoreOS() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

// GoVersion 返回编译用的 Go 工具链版本，如 "go1.26.5"
func GoVersion() string {
	return runtime.Version()
}
