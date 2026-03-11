package wallpaper

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrEmptyFilename       = errors.New("empty filename provided")
	ErrFileNotExist        = errors.New("file does not exist")
	ErrUnsupportedDesktop  = errors.New("unsupported desktop environment")
	ErrSetCommandFailed    = errors.New("error setting wallpaper with command")
	ErrUnsupportedFiletype = errors.New("unspported filetype")
)

func Set(filename string) error {
	if filename == "" {
		return ErrEmptyFilename
	}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrFileNotExist, filename)
	}

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp":
		break
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedFiletype, ext)
	}

	err := setWallpaper(filename)
	if err != nil {
		return err
	}

	return nil
}
