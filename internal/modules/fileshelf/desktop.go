package fileshelf

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Driver interface {
	CopyPath(context.Context, string) error
	OpenFolder(context.Context, string) error
}
type Desktop struct{}

func (Desktop) CopyPath(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "wl-copy", "--type", "text/plain;charset=utf-8")
	cmd.Stdin = strings.NewReader(path)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not copy path; check wl-clipboard and Wayland")
	}
	return nil
}
func (Desktop) OpenFolder(ctx context.Context, path string) error {
	// Only directory paths are passed as a single argument, without a shell.
	// Command output is not logged because desktop helpers may print private paths.
	if err := exec.CommandContext(ctx, "xdg-open", path).Run(); err != nil {
		return fmt.Errorf("could not open folder; check xdg-utils and your file manager")
	}
	return nil
}
