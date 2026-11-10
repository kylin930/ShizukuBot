//go:build windows

package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// createNoWindow 让后台脚本不弹出控制台窗口。
const createNoWindow = 0x08000000

// Restart 在当前进程退出后，用 staged 替换 executable 并重新启动。
// pid 是当前主程序进程号，脚本会等到它退出后再动文件。
func Restart(staged, executable string, pid int) error {
	if !Exists(staged) {
		return fmt.Errorf("构建产物不存在：%s", staged)
	}
	workDir, err := os.Getwd()
	if err != nil {
		workDir = filepath.Dir(executable)
	}
	failure := filepath.Join(workDir, "reload-failed.txt")
	script := filepath.Join(os.TempDir(), fmt.Sprintf("shizukubot-reload-%d.bat", pid))

	// move 在旧进程完全退出前可能仍被系统占用，最多重试 30 次。
	body := fmt.Sprintf(`@echo off
setlocal
:wait
tasklist /FI "PID eq %[1]d" /NH 2>nul | findstr /C:"%[1]d" >nul
if not errorlevel 1 (
  ping -n 2 127.0.0.1 >nul
  goto wait
)
set tries=0
:replace
move /y "%[2]s" "%[3]s" >nul 2>nul
if not errorlevel 1 goto launch
set /a tries+=1
if %[4]d leq %%tries%% (
  echo ShizukuBot reload failed: cannot replace "%[3]s" with "%[2]s".>"%[5]s"
  exit /b 1
)
ping -n 2 127.0.0.1 >nul
goto replace
:launch
start "" /D "%[6]s" "%[3]s"
del "%%~f0"
`, pid, staged, executable, 30, failure, workDir)

	if err := os.WriteFile(script, []byte(body), 0600); err != nil {
		return err
	}
	cmd := exec.Command("cmd.exe", "/C", filepath.Base(script))
	// 以脚本所在目录为工作目录，避免临时目录含空格时 cmd /C 的引号解析问题。
	cmd.Dir = filepath.Dir(script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		_ = os.Remove(script)
		return err
	}
	go func() {
		_ = cmd.Wait()
		_ = os.Remove(script)
	}()
	return nil
}

// Delay 给 HTTP 响应留出写回时间。
func Delay() { time.Sleep(750 * time.Millisecond) }
