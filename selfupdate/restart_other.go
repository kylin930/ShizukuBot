//go:build !windows

package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Restart 在当前进程退出后，用 staged 替换 executable 并重新启动。
func Restart(staged, executable string, pid int) error {
	if !Exists(staged) {
		return fmt.Errorf("构建产物不存在：%s", staged)
	}
	workDir, err := os.Getwd()
	if err != nil {
		workDir = filepath.Dir(executable)
	}
	script := filepath.Join(os.TempDir(), fmt.Sprintf("shizukubot-reload-%d.sh", pid))
	body := fmt.Sprintf(`#!/bin/sh
while kill -0 %[1]d 2>/dev/null; do sleep 1; done
mv -f '%[2]s' '%[3]s' || exit 1
cd '%[4]s' || exit 1
rm -f '%[5]s'
exec '%[3]s'
`, pid, staged, executable, workDir, script)
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		return err
	}
	cmd := exec.Command("/bin/sh", script)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		_ = os.Remove(script)
		return err
	}
	return cmd.Process.Release()
}

// Delay 给 HTTP 响应留出写回时间。
func Delay() { time.Sleep(750 * time.Millisecond) }
