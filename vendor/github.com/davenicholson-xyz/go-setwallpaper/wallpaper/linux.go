//go:build linux
// +build linux

package wallpaper

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var commands = map[string]string{
	"plasma":         "dbus-send --session --dest=org.kde.plasmashell --type=method_call /PlasmaShell org.kde.PlasmaShell.evaluateScript string:\"var allDesktops = desktops(); for (i = 0; i < allDesktops.length; i++) { d = allDesktops[i]; d.wallpaperPlugin = 'org.kde.image'; d.currentConfigGroup = Array('Wallpaper', 'org.kde.image', 'General'); d.writeConfig('Image', 'file://{IMG}'); }\"",
	"gnome":          "gsettings set org.gnome.desktop.background picture-uri file://{IMG} && gsettings set org.gnome.desktop.background picture-uri-dark file://{IMG}",
	"gnome-wayland":  "gsettings set org.gnome.desktop.background picture-uri file://{IMG} && gsettings set org.gnome.desktop.background picture-uri-dark file://{IMG}",
	"ubuntu":         "gsettings set org.gnome.desktop.background picture-uri file://{IMG} && gsettings set org.gnome.desktop.background picture-uri-dark file://{IMG}",
	"cinnamon":       "gsettings set org.cinnamon.desktop.background picture-uri file://{IMG}",
	"mate":           "gsettings set org.mate.background picture-filename \"{IMG}\"",
	"budgie-desktop": "gsettings set org.gnome.desktop.background picture-uri \"file://{IMG}\"",
	"xfce":           "for prop in $(xfconf-query -c xfce4-desktop -l | grep last-image); do xfconf-query -c xfce4-desktop -p $prop -s '{IMG}'; done",
}

func setWallpaper(path string) error {

	desktop := os.Getenv("DESKTOP_SESSION")

	cmd, exists := commands[desktop]
	if !exists {
		return fmt.Errorf("%w: %s", ErrUnsupportedDesktop, desktop)
	}

	cmdString := strings.ReplaceAll(cmd, "{IMG}", path)

	output, err := exec.Command("sh", "-c", cmdString).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrSetCommandFailed, output)
	}
	return nil
}
