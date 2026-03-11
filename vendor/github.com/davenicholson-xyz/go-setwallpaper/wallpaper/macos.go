//go:build darwin
// +build darwin

package wallpaper

import (
	"fmt"
	"os/exec"
	"strings"
)

func setWallpaper(path string) error {
	cmd := "osascript -e 'tell application \"System Events\" to set picture of every desktop to POSIX file \"{IMG}\"'"

	cmdString := strings.ReplaceAll(cmd, "{IMG}", path)

	output, err := exec.Command("sh", "-c", cmdString).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrSetCommandFailed, output)
	}
	return nil
}
