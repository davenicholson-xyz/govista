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
	flag.StringVar(&cfg.APIKey, "k", cfg.APIKey, "Wallhaven API key")
	flag.StringVar(&cfg.APIKey, "api-key", cfg.APIKey, "Wallhaven API key")
	flag.StringVar(&cfg.Username, "u", cfg.Username, "Wallhaven username")
	flag.StringVar(&cfg.Username, "username", cfg.Username, "Wallhaven username")
	flag.StringVar(&cfg.Query, "q", cfg.Query, "Default search query")
	flag.StringVar(&cfg.Query, "query", cfg.Query, "Default search query")
	flag.StringVar(&cfg.Categories, "c", cfg.Categories, "Category bitmask (e.g. 111)")
	flag.StringVar(&cfg.Categories, "categories", cfg.Categories, "Category bitmask (e.g. 111)")
	flag.StringVar(&cfg.Purity, "p", cfg.Purity, "Purity bitmask (e.g. 100)")
	flag.StringVar(&cfg.Purity, "purity", cfg.Purity, "Purity bitmask (e.g. 100)")
	flag.StringVar(&cfg.MinResolution, "r", cfg.MinResolution, "Minimum resolution (e.g. 1920x1080)")
	flag.StringVar(&cfg.MinResolution, "min-resolution", cfg.MinResolution, "Minimum resolution (e.g. 1920x1080)")
	flag.BoolVar(&cfg.CloseOnSelect, "x", cfg.CloseOnSelect, "Close window after selecting a wallpaper")
	flag.BoolVar(&cfg.CloseOnSelect, "close-on-select", cfg.CloseOnSelect, "Close window after selecting a wallpaper")
	flag.BoolVar(&cfg.Output, "o", cfg.Output, "Print selected wallpaper path to stdout")
	flag.BoolVar(&cfg.Output, "output", cfg.Output, "Print selected wallpaper path to stdout")
	flag.StringVar(&cfg.Script, "s", cfg.Script, "Script to run on selected wallpaper (receives filepath as argument)")
	flag.StringVar(&cfg.Script, "script", cfg.Script, "Script to run on selected wallpaper (receives filepath as argument)")

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
