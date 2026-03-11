package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	wh "github.com/davenicholson-xyz/go-wallhaven/wallhavenapi"
	"github.com/davenicholson-xyz/go-setwallpaper/wallpaper"
	"gioui.org/app"
	"gioui.org/io/system"
)

// newQuery creates a reusable search query for the latest wallpapers.
func newQuery() *wh.Query {
	return wh.New().Search("").Sort(wh.DateAdded)
}

// fetchPage retrieves a specific page of wallpapers and returns the thumbs
// along with the last available page number from the API metadata.
func fetchPage(q *wh.Query, page int) ([]*Thumb, int, error) {
	result, err := q.Page(page)
	if err != nil {
		return nil, 0, fmt.Errorf("wallhaven page %d: %w", page, err)
	}

	thumbs := make([]*Thumb, 0, len(result.Wallpapers))
	for _, w := range result.Wallpapers {
		thumbs = append(thumbs, &Thumb{
			ID:       w.ID,
			ThumbURL: w.Thumbs.Large,
			FullURL:  w.Path,
		})
	}
	return thumbs, result.Meta.LastPage, nil
}

// cacheDir returns (and creates if needed) ~/.cache/govista.
func cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "govista")
	return dir, os.MkdirAll(dir, 0755)
}

// downloadAndSet downloads the full-resolution wallpaper identified by id at
// url into the cache folder, sets it as the desktop wallpaper, prints the
// cached path to stdout, and closes the window.
func downloadAndSet(id, url string, w *app.Window) error {
	dir, err := cacheDir()
	if err != nil {
		return err
	}

	// Derive extension from URL; fall back to .jpg.
	ext := filepath.Ext(url)
	if ext == "" {
		ext = ".jpg"
	}
	dest := filepath.Join(dir, id+ext)

	// Re-use cached file if already downloaded.
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		f, err := os.Create(dest)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, resp.Body); err != nil {
			f.Close()
			os.Remove(dest)
			return err
		}
		if err := f.Close(); err != nil {
			os.Remove(dest)
			return err
		}
	}

	err = wallpaper.Set(dest)
	fmt.Println(dest)
	w.Perform(system.ActionClose)
	return err
}
