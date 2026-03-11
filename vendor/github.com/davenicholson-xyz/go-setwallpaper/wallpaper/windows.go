//go:build windows
// +build windows

package wallpaper

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"syscall"
	"unsafe"
)

const (
	SPI_SETDESKWALLPAPER = 0x0014
	SPIF_UPDATEINIFILE   = 0x01
	SPIF_SENDCHANGE      = 0x02
)

func setWallpaper(path string) error {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Control Panel\Desktop`,
		registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer key.Close()

	err = key.SetStringValue("WallpaperStyle", "10")
	if err != nil {
		return err
	}

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}

	user32 := windows.NewLazySystemDLL("user32.dll")
	systemParametersInfo := user32.NewProc("SystemParametersInfoW")

	ret, _, err := systemParametersInfo.Call(
		uintptr(SPI_SETDESKWALLPAPER),
		uintptr(0),
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(SPIF_UPDATEINIFILE|SPIF_SENDCHANGE),
	)

	if ret == 0 {
		return err
	}

	return nil
}
