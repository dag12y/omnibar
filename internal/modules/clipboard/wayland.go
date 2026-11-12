package clipboard

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type Driver interface {
	Watch(context.Context, func(Capture)) error
	Copy(context.Context, string) error
}
type Wayland struct{}

func (Wayland) Watch(ctx context.Context, emit func(Capture)) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("clipboard helper is unavailable")
	}
	cmd := exec.CommandContext(ctx, "wl-paste", "--type", "text", "--watch", executable, "--clipboard-capture")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	// stderr goes to /dev/null; clipboard contents are never diagnostic output.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("clipboard watching needs wl-clipboard and a Wayland session")
	}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), MaxItemBytes*8)
	for scanner.Scan() {
		var capture Capture
		if json.Unmarshal(scanner.Bytes(), &capture) == nil && ctx.Err() == nil {
			emit(capture)
		}
	}
	if scanner.Err() != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	_ = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("clipboard watcher unavailable; check Wayland data-control support")
}
func (Wayland) Copy(ctx context.Context, text string) error {
	cmd := exec.CommandContext(ctx, "wl-copy", "--type", "text/plain;charset=utf-8")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not copy text to the Wayland clipboard")
	}
	return nil
}
