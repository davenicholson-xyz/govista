package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	wh "github.com/davenicholson-xyz/go-wallhaven/wallhavenapi"
	"github.com/davenicholson-xyz/go-setwallpaper/wallpaper"
	"gioui.org/app"
	"gioui.org/io/system"
)

// fetchCollections returns the user's collections. Uses MyCollections (private+public)
// when an API key is set, otherwise falls back to public Collections by username.
func fetchCollections(cfg Config) ([]wh.Collection, error) {
	var client *wh.WallhavenAPI
	if cfg.APIKey != "" {
		client = wh.NewWithAPIKey(cfg.APIKey)
	} else {
		client = wh.New()
	}
	if cfg.APIKey != "" {
		return client.MyCollections()
	}
	if cfg.Username != "" {
		return client.Collections(cfg.Username)
	}
	return nil, fmt.Errorf("api key or username required for collections")
}

// buildCollectionQuery returns a query for wallpapers in a specific collection.
func buildCollectionQuery(cfg Config, username string, id int) *wh.Query {
	var client *wh.WallhavenAPI
	if cfg.APIKey != "" {
		client = wh.NewWithAPIKey(cfg.APIKey)
	} else {
		client = wh.New()
	}
	return client.Collection(username, id)
}

// buildQuery constructs a Wallhaven search query from the current config and
// active state (sorting mode, search text, random seed).
func buildQuery(cfg Config, sorting, query, seed string) *wh.Query {
	var client *wh.WallhavenAPI
	if cfg.APIKey != "" {
		client = wh.NewWithAPIKey(cfg.APIKey)
	} else {
		client = wh.New()
	}

	q := client.Search(query)

	switch sorting {
	case "hot":
		q = q.Sort(wh.Hot)
	case "toplist":
		q = q.Sort(wh.Toplist)
	case "random":
		q = q.Sort(wh.Random)
	default:
		q = q.Sort(wh.DateAdded)
	}

	if cats := parseCategoriesFlags(cfg.Categories); len(cats) > 0 {
		q = q.Categories(cats...)
	}
	if purs := parsePurityFlags(cfg.Purity); len(purs) > 0 {
		q = q.Purity(purs...)
	}
	if cfg.MinResolution != "" {
		q = q.MinimumResolution(cfg.MinResolution)
	}
	if seed != "" {
		q = q.Seed(seed)
	}
	return q
}

// parseCategoriesFlags converts a 3-char bitmask string (e.g. "111") into
// CategoriesFlag values (General, Anime, People).
func parseCategoriesFlags(mask string) []wh.CategoriesFlag {
	var flags []wh.CategoriesFlag
	if len(mask) > 0 && mask[0] == '1' {
		flags = append(flags, wh.General)
	}
	if len(mask) > 1 && mask[1] == '1' {
		flags = append(flags, wh.Anime)
	}
	if len(mask) > 2 && mask[2] == '1' {
		flags = append(flags, wh.People)
	}
	return flags
}

// parsePurityFlags converts a 3-char bitmask string (e.g. "100") into
// PurityFlag values (SFW, Sketchy, NSFW).
func parsePurityFlags(mask string) []wh.PurityFlag {
	var flags []wh.PurityFlag
	if len(mask) > 0 && mask[0] == '1' {
		flags = append(flags, wh.SFW)
	}
	if len(mask) > 1 && mask[1] == '1' {
		flags = append(flags, wh.Sketchy)
	}
	if len(mask) > 2 && mask[2] == '1' {
		flags = append(flags, wh.NSFW)
	}
	return flags
}

// fetchPage retrieves a specific page of wallpapers and returns (thumbs, lastPage, error).
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

// thumbCacheDir returns (and creates if needed) ~/.cache/govista/thumbs.
func thumbCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "govista", "thumbs")
	return dir, os.MkdirAll(dir, 0755)
}

// downloadAndSet downloads the full-resolution wallpaper, sets it as the
// desktop wallpaper, and — depending on config — prints the path and/or
// closes the window.
func downloadAndSet(id, thumbURL, url string, cfg Config, w *app.Window) error {
	dir, err := cacheDir()
	if err != nil {
		return err
	}

	ext := filepath.Ext(url)
	if ext == "" {
		ext = ".jpg"
	}
	dest := filepath.Join(dir, id+ext)

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

	if cfg.Script != "" {
		if err := exec.Command(cfg.Script, dest).Run(); err != nil {
			return fmt.Errorf("script: %w", err)
		}
	} else {
		if err := wallpaper.Set(dest); err != nil {
			return err
		}
	}
	// Record in history (async — don't delay the wallpaper set).
	go appendHistoryEntry(HistoryEntry{ID: id, ThumbURL: thumbURL, FullURL: url})

	if cfg.Output {
		fmt.Println(dest)
	}
	if cfg.CloseOnSelect {
		w.Perform(system.ActionClose)
	}
	return nil
}
