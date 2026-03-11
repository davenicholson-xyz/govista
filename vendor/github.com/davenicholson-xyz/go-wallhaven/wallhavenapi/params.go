package wallhavenapi

import (
	"fmt"
	"strconv"
	"strings"
)

// APIKey sets the API key for authenticated requests to the Wallhaven API.
// The API key is required for accessing NSFW content and user-specific data.
func (wh *WallhavenAPI) APIKey(apikey string) {
	wh.urlbuilder.SetString("apikey", apikey)
}

// Categories sets the wallpaper categories to search within.
// Accepts one or more CategoriesFlag values (e.g., General, Anime, People).
// Multiple flags can be combined to search across multiple categories.
func (wh *WallhavenAPI) Categories(flags ...CategoriesFlag) {
	categoriesString := CategoriesFlagToString(flags...)
	wh.urlbuilder.SetString("categories", categoriesString)
}

// Categories sets the wallpaper categories to search within inline.
// Accepts one or more CategoriesFlag values (e.g., General, Anime, People).
// Multiple flags can be combined to search across multiple categories.
func (q *Query) Categories(flags ...CategoriesFlag) *Query {
	categoriesString := CategoriesFlagToString(flags...)
	q.URLBuilder.SetString("categories", categoriesString)
	return q
}

// Purity sets the content purity levels to include in search results.
// Accepts one or more PurityFlag values (e.g., SFW, Sketchy, NSFW).
// Multiple flags can be combined to include multiple purity levels.
func (wh *WallhavenAPI) Purity(flags ...PurityFlag) {
	purityString := PurityFlagToString(flags...)
	wh.urlbuilder.SetString("purity", purityString)
}

// Purity sets the content purity levels to include in search results inline.
// Accepts one or more PurityFlag values (e.g., SFW, Sketchy, NSFW).
// Multiple flags can be combined to include multiple purity levels.
func (q *Query) Purity(flags ...PurityFlag) *Query {
	purityString := PurityFlagToString(flags...)
	q.URLBuilder.SetString("purity", purityString)
	return q
}

// PurityMask sets the purity filter using a bitmask.
// The mask parameter represents a binary combination of purity flags.
// This provides more granular control over purity filtering than the Purity method.
func (wh *WallhavenAPI) PurityMask(mask PurityFlag) {
	result := fmt.Sprintf("%03s", strconv.FormatInt(int64(mask), 2))
	wh.urlbuilder.SetString("purity", result)
}

// Sort sets the sorting method for search results.
// Accepts a SortingType value such as Date, Random, Views, Favorites, etc.
func (wh *WallhavenAPI) Sort(sort SortingType) {
	wh.urlbuilder.SetString("sorting", string(sort))
}

// Sort sets the sorting method for search results inline.
// Accepts a SortingType value such as Date, Random, Views, Favorites, etc.
func (q *Query) Sort(sort SortingType) *Query {
	q.URLBuilder.SetString("sorting", string(sort))
	return q
}

// Order sets the sort order for search results.
// Accepts an OrderType value (typically Ascending or Descending).
func (wh *WallhavenAPI) Order(order OrderType) {
	wh.urlbuilder.SetString("order", string(order))
}

// Order sets the sort order for search results inline.
// Accepts an OrderType value (typically Ascending or Descending).
func (q *Query) Order(order OrderType) *Query {
	q.URLBuilder.SetString("order", string(order))
	return q
}

// Range sets the time range for "top" sorting.
// Accepts a RangeType value such as Day, Week, Month, or Year.
// This parameter is only relevant when using "top" as the sorting method.
func (wh *WallhavenAPI) Range(rng RangeType) {
	wh.urlbuilder.SetString("topRange", string(rng))
}

// Range sets the time range for "top" sorting inline.
// Accepts a RangeType value such as Day, Week, Month, or Year.
// This parameter is only relevant when using "top" as the sorting method.
func (q *Query) Range(rng RangeType) *Query {
	q.URLBuilder.SetString("topRange", string(rng))
	return q
}

// MinimumResolution sets the minimum resolution filter for wallpapers.
// The res parameter should be in the format "1920x1080" or similar.
// Only wallpapers with resolution equal to or greater than this will be returned.
func (wh *WallhavenAPI) MinimumResolution(res string) {
	wh.urlbuilder.SetString("atleast", res)
}

// MinimumResolution sets the minimum resolution filter for wallpapers inline.
// The res parameter should be in the format "1920x1080" or similar.
// Only wallpapers with resolution equal to or greater than this will be returned.
func (q *Query) MinimumResolution(res string) *Query {
	q.URLBuilder.SetString("atleast", res)
	return q
}

// Seed sets a seed value for consistent random results.
// When using random sorting, the same seed will produce the same order of results.
// This is useful for pagination with random sorting.
func (wh *WallhavenAPI) Seed(seed string) {
	wh.urlbuilder.SetString("seed", seed)
}

// Seed sets a seed value for consistent random results inline.
// When using random sorting, the same seed will produce the same order of results.
// This is useful for pagination with random sorting.
func (q *Query) Seed(seed string) *Query {
	q.URLBuilder.SetString("seed", seed)
	return q
}

// Colors filters wallpapers by dominant color.
// The hex parameter should be a color in hexadecimal format (e.g., "ff0000" for red).
// Do not include the "#" prefix in the hex value.
func (wh *WallhavenAPI) Colors(hex string) {
	wh.urlbuilder.SetString("colors", hex)
}

// Colors filters wallpapers by dominant color inline.
// The hex parameter should be a color in hexadecimal format (e.g., "ff0000" for red).
// Do not include the "#" prefix in the hex value.
func (q *Query) Colors(hex string) *Query {
	q.URLBuilder.SetString("colors", hex)
	return q
}

// Resolutions filters wallpapers by specific resolutions.
// Accepts one or more resolution strings in the format "1920x1080".
// Multiple resolutions are combined with OR logic (wallpapers matching any resolution).
func (wh *WallhavenAPI) Resolutions(res ...string) {
	joined := strings.Join(res, ",")
	wh.urlbuilder.SetString("resolutions", joined)
}

// Resolutions filters wallpapers by specific resolutions inline.
// Accepts one or more resolution strings in the format "1920x1080".
// Multiple resolutions are combined with OR logic (wallpapers matching any resolution).
func (q *Query) Resolutions(res ...string) *Query {
	joined := strings.Join(res, ",")
	q.URLBuilder.SetString("resolutions", joined)
	return q
}

// Ratios filters wallpapers by aspect ratios.
// Accepts one or more ratio strings in the format "16x9" or "4x3".
// Multiple ratios are combined with OR logic (wallpapers matching any ratio).
func (wh *WallhavenAPI) Ratios(ratios ...string) {
	joined := strings.Join(ratios, ",")
	wh.urlbuilder.SetString("ratios", joined)
}

// Ratios filters wallpapers by aspect ratios inline.
// Accepts one or more ratio strings in the format "16x9" or "4x3".
// Multiple ratios are combined with OR logic (wallpapers matching any ratio).
func (q *Query) Ratios(ratios ...string) *Query {
	joined := strings.Join(ratios, ",")
	q.URLBuilder.SetString("ratios", joined)
	return q
}
