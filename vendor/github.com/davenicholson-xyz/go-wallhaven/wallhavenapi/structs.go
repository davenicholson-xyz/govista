package wallhavenapi

import (
	"fmt"
	"strconv"
)

type WallpaperQueryData struct {
	Data Wallpaper `json:"data"`
}

type SearchQueryData struct {
	Wallpapers []Wallpaper `json:"data"`
	Meta       struct {
		CurrentPage int `json:"current_page"`
		LastPage    int `json:"last_page"`
		Total       int `json:"total"`
		PerPage     int
		Query       string `json:"query"`
		Seed        string `json:"seed"`
	} `json:"meta"`
}

type Wallpaper struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	ShortURL string `json:"short_url"`
	Uploader struct {
		Username string `json:"username"`
		Group    string `json:"group"`
		Avatar   struct {
			Size200px string `json:"200px"`
			Size128px string `json:"128px"`
			Size32px  string `json:"32px"`
			Size20px  string `json:"20px"`
		} `json:"avatar"`
	} `json:"uploader"`
	Views      int      `json:"views"`
	Favorites  int      `json:"favorites"`
	Source     string   `json:"source"`
	Purity     string   `json:"purity"`
	Category   string   `json:"category"`
	DimensionX int      `json:"dimension_x"`
	DimensionY int      `json:"dimension_y"`
	Resolution string   `json:"resolution"`
	Ratio      string   `json:"ratio"`
	FileSize   int      `json:"file_size"`
	FileType   string   `json:"file_type"`
	CreatedAt  string   `json:"created_at"`
	Colors     []string `json:"colors"`
	Path       string   `json:"path"`
	Thumbs     struct {
		Large    string `json:"large"`
		Original string `json:"original"`
		Small    string `json:"small"`
	} `json:"thumbs"`
	Tags []struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		Alias      string `json:"alias"`
		CategoryID int    `json:"category_id"`
		Category   string `json:"category"`
		Purity     string `json:"purity"`
		CreatedAt  string `json:"created_at"`
	} `json:"tags"`
}

type TagData struct {
	Data Tag `json:"data"`
}

type Tag struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Alias      string `json:"alias"`
	CategoryID int    `json:"category_id"`
	Category   string `json:"category"`
	Purity     string `json:"purity"`
	CreatedAt  string `json:"created_at"`
}

type UserSettingsData struct {
	Data UserSettings `json:"data"`
}

type UserSettings struct {
	ThumbSize     string   `json:"thumb_size"`
	PerPage       string   `json:"per_page"`
	Purity        []string `json:"purity"`
	Categories    []string `json:"categories"`
	Resolutions   []string `json:"resolutions"`
	AspectRatios  []string `json:"aspect_ratios"`
	ToplistRange  string   `json:"toplist_range"`
	TagBlacklist  []string `json:"tag_blacklist"`
	UserBlacklist []string `json:"user_blacklist"`
}

type CollectionData struct {
	Data []Collection `json:"data"`
}

type Collection struct {
	ID     int    `json:"id"`
	Label  string `json:"label"`
	Views  int    `json:"views"`
	Public int    `json:"public"`
	Count  int    `json:"count"`
}

type CollectionQueryData struct {
	Data []Wallpaper `json:"data"`
	Meta struct {
		CurrentPage int    `json:"current_page"`
		LastPage    int    `json:"last_page"`
		PerPage     string `json:"per_page"`
		Total       int    `json:"total"`
	} `json:"meta"`
}

type SortingType string

const (
	DateAdded SortingType = "date_added"
	Relevance SortingType = "relevance"
	Random    SortingType = "random"
	Views     SortingType = "views"
	Favorites SortingType = "favorites"
	Toplist   SortingType = "toplist"
	Hot       SortingType = "hot"
)

type OrderType string

const (
	Descending OrderType = "desc"
	Ascending  OrderType = "asc"
)

type RangeType string

const (
	OneDay      RangeType = "1d"
	ThreeDays   RangeType = "3d"
	OneWeek     RangeType = "1w"
	OneMonth    RangeType = "1M"
	ThreeMonths RangeType = "3M"
	SixMonths   RangeType = "6M"
	OneYear     RangeType = "1y"
)

const (
	SFW     = 1 << 2
	Sketchy = 1 << 1
	NSFW    = 1 << 0
)

type PurityFlag int

func PurityFlagToString(flags ...PurityFlag) string {
	var combined int
	for _, flag := range flags {
		combined |= int(flag)
	}
	result := fmt.Sprintf("%03s", strconv.FormatInt(int64(combined), 2))
	return result
}

const (
	General = 1 << 2
	Anime   = 1 << 1
	People  = 1 << 0
)

type CategoriesFlag int

func CategoriesFlagToString(flags ...CategoriesFlag) string {
	var combined int
	for _, flag := range flags {
		combined |= int(flag)
	}
	result := fmt.Sprintf("%03s", strconv.FormatInt(int64(combined), 2))
	return result
}
