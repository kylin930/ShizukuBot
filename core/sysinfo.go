package core

import (
	"fmt"
	"os"
	"sync"

	"github.com/shirou/gopsutil/v3/process"
)

var (
	procOnce sync.Once
	proc     *process.Process
	procErr  error
)

// getProc 单例获取当前进程句柄。
// process.CPUPercent 依赖上次采样点，首次调用返回 0；
// 这里在包初始化后第一次调用时预热，保证后续 /status 能拿到有意义值。
func getProc() (*process.Process, error) {
	procOnce.Do(func() {
		proc, procErr = process.NewProcess(int32(os.Getpid()))
	})
	return proc, procErr
}

// CoreCPUPercent 返回当前进程 CPU 占用率（百分比，1位小数）。
// 获取失败返回 "N/A"。
func CoreCPUPercent() string {
	p, err := getProc()
	if err != nil {
		return "N/A"
	}
	pct, err := p.CPUPercent()
	if err != nil {
		return "N/A"
	}
	return fmt.Sprintf("%.1f%%", pct)
}

// CoreProcMemRSS 返回当前进程驻留内存（RSS，实际占用），单位 MB（1位小数）。
// 获取失败返回 "N/A"。
func CoreProcMemRSS() string {
	p, err := getProc()
	if err != nil {
		return "N/A"
	}
	mi, err := p.MemoryInfo()
	if err != nil || mi == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.1f MB", float64(mi.RSS)/1024/1024)
}
