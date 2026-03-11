package main

import (
	"flag"
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
	MinResolution string `toml:"min-resolution"`
	Script        string `toml:"script"`
	CloseOnSelect bool   `toml:"close-on-select"`
	Output        bool   `toml:"output"`
}

func newDefaultConfig() Config {
	return Config{
		Categories: "111",
		Purity:     "100",
		Sorting:    "date_added",
	}
}

// loadConfig reads ~/.config/govista/config.toml, falling back to defaults.
func loadConfig() Config {
	cfg := newDefaultConfig()
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}
	path := filepath.Join(home, ".config", "govista", "config.toml")
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
	var closeOnSelect, output bool

	using := func(v string) string {
		if v != "" {
			return "using: " + v
		}
		return ""
	}

	flag.StringVar(&apiKey, "k", "", "Wallhaven API key ("+using(cfg.APIKey)+")")
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
	flag.BoolVar(&closeOnSelect, "x", false, "Close window after selecting a wallpaper")
	flag.BoolVar(&closeOnSelect, "close-on-select", false, "Close window after selecting a wallpaper")
	flag.BoolVar(&output, "o", false, "Print selected wallpaper path to stdout")
	flag.BoolVar(&output, "output", false, "Print selected wallpaper path to stdout")
	flag.StringVar(&script, "s", "", "Script to run on selected wallpaper ("+using(cfg.Script)+")")
	flag.StringVar(&script, "script", "", "Script to run on selected wallpaper ("+using(cfg.Script)+")")

	var hot, top, latest, random bool
	flag.BoolVar(&hot, "H", false, "Start with Hot sorting")
	flag.BoolVar(&hot, "hot", false, "Start with Hot sorting")
	flag.BoolVar(&top, "T", false, "Start with Toplist sorting")
	flag.BoolVar(&top, "top", false, "Start with Toplist sorting")
	flag.BoolVar(&latest, "l", false, "Start with Latest sorting")
	flag.BoolVar(&latest, "latest", false, "Start with Latest sorting")
	flag.BoolVar(&random, "R", false, "Start with Random sorting")
	flag.BoolVar(&random, "random", false, "Start with Random sorting")

	flag.Parse()

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
	if closeOnSelect {
		cfg.CloseOnSelect = true
	}
	if output {
		cfg.Output = true
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
