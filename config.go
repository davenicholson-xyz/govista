package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all application configuration loaded from file and/or CLI flags.
type Config struct {
	APIKey        string `toml:"api_key"`
	Username      string `toml:"username"`
	Query         string `toml:"query"`
	Categories    string `toml:"categories"`
	Purity        string `toml:"purity"`
	Sorting       string `toml:"sorting"`
	SearchSorting string `toml:"search-sorting"`
	MinResolution string `toml:"min-resolution"`
	Script        string `toml:"script"`
	CloseOnSelect bool   `toml:"close-on-select"`
	Output        bool   `toml:"output"`
	ThumbSize     int    `toml:"thumb-size"`
	CacheMaxMB    int    `toml:"cache-max-mb"`
}

func newDefaultConfig() Config {
	return Config{
		Categories:    "111",
		Purity:        "100",
		Sorting:       "date_added",
		SearchSorting: "relevance",
		ThumbSize:     200,
		CloseOnSelect: true,
		CacheMaxMB:    500,
	}
}

// loadConfig reads the platform config dir for govista/config.toml, falling back to defaults.
// On Linux/macOS this is ~/.config/govista/config.toml; on Windows %AppData%\govista\config.toml.
func loadConfig() Config {
	cfg := newDefaultConfig()
	configDir, err := os.UserConfigDir()
	if err != nil {
		return cfg
	}
	path := filepath.Join(configDir, "govista", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		log.Println("govista: config parse error:", err)
	}
	return cfg
}

// parseFlags overlays CLI flags on top of the loaded config. Flags take precedence.
func parseFlags(cfg *Config) {
	var apiKey, username, query, categories, purity, minRes, script string
	var keepOpen, output bool
	var thumbSize int

	using := func(v string) string {
		if v != "" {
			return "using: " + v
		}
		return ""
	}

	flag.StringVar(&apiKey, "a", "", "Wallhaven API key ("+using(cfg.APIKey)+")")
	flag.StringVar(&apiKey, "api-key", "", "Wallhaven API key ("+using(cfg.APIKey)+")")
	flag.StringVar(&username, "u", "", "Wallhaven username ("+using(cfg.Username)+")")
	flag.StringVar(&username, "username", "", "Wallhaven username ("+using(cfg.Username)+")")
	flag.StringVar(&query, "q", "", "Default search query ("+using(cfg.Query)+")")
	flag.StringVar(&query, "query", "", "Default search query ("+using(cfg.Query)+")")
	flag.StringVar(&categories, "c", "", "Category bitmask ("+using(cfg.Categories)+")")
	flag.StringVar(&categories, "categories", "", "Category bitmask ("+using(cfg.Categories)+")")
	flag.StringVar(&purity, "p", "", "Purity bitmask ("+using(cfg.Purity)+")")
	flag.StringVar(&purity, "purity", "", "Purity bitmask ("+using(cfg.Purity)+")")
	flag.StringVar(&minRes, "r", "", "Minimum resolution e.g. 1920x1080 ("+using(cfg.MinResolution)+")")
	flag.StringVar(&minRes, "min-resolution", "", "Minimum resolution e.g. 1920x1080 ("+using(cfg.MinResolution)+")")
	flag.BoolVar(&keepOpen, "k", false, "Keep window open after selecting a wallpaper")
	flag.BoolVar(&keepOpen, "keep-open", false, "Keep window open after selecting a wallpaper")
	flag.BoolVar(&output, "o", false, "Print selected wallpaper path to stdout")
	flag.BoolVar(&output, "output", false, "Print selected wallpaper path to stdout")
	flag.IntVar(&thumbSize, "t", 0, fmt.Sprintf("Thumbnail size in dp (default %d)", cfg.ThumbSize))
	flag.IntVar(&thumbSize, "thumb-size", 0, fmt.Sprintf("Thumbnail size in dp (default %d)", cfg.ThumbSize))
	flag.StringVar(&script, "s", "", "Script to run on selected wallpaper ("+using(cfg.Script)+")")
	flag.StringVar(&script, "script", "", "Script to run on selected wallpaper ("+using(cfg.Script)+")")

	var cacheMaxMB int
	flag.IntVar(&cacheMaxMB, "cache-max-mb", 0, fmt.Sprintf("Max cache size in MB, 0=unlimited (default %d)", cfg.CacheMaxMB))

	var hot, top, latest, random bool
	flag.BoolVar(&hot, "H", false, "Start with Hot sorting")
	flag.BoolVar(&hot, "hot", false, "Start with Hot sorting")
	flag.BoolVar(&top, "T", false, "Start with Toplist sorting")
	flag.BoolVar(&top, "top", false, "Start with Toplist sorting")
	flag.BoolVar(&latest, "l", false, "Start with Latest sorting")
	flag.BoolVar(&latest, "latest", false, "Start with Latest sorting")
	flag.BoolVar(&random, "R", false, "Start with Random sorting")
	flag.BoolVar(&random, "random", false, "Start with Random sorting")

	var showVersion bool
	flag.BoolVar(&showVersion, "v", false, "Print version and exit")
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")

	flag.Parse()

	if showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	if apiKey != "" {
		cfg.APIKey = apiKey
	}
	if username != "" {
		cfg.Username = username
	}
	if query != "" {
		cfg.Query = query
	}
	if categories != "" {
		cfg.Categories = categories
	}
	if purity != "" {
		cfg.Purity = purity
	}
	if minRes != "" {
		cfg.MinResolution = minRes
	}
	if script != "" {
		cfg.Script = script
	}
	if keepOpen {
		cfg.CloseOnSelect = false
	}
	if output {
		cfg.Output = true
	}
	if thumbSize > 0 {
		cfg.ThumbSize = thumbSize
	}
	if cacheMaxMB > 0 {
		cfg.CacheMaxMB = cacheMaxMB
	}

	switch {
	case hot:
		cfg.Sorting = "hot"
	case top:
		cfg.Sorting = "toplist"
	case latest:
		cfg.Sorting = "date_added"
	case random:
		cfg.Sorting = "random"
	}
}
