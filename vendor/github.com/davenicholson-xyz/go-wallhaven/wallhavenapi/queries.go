package wallhavenapi

import (
	"fmt"

	"github.com/davenicholson-xyz/go-wallhaven/wallhavenapi/fetch"
)

// Query represents a query builder for Wallhaven API search operations.
// It wraps a URLBuilder to construct and execute search queries with pagination support.
type Query struct {
	*fetch.URLBuilder
}

// Wallpaper retrieves a specific wallpaper by its ID.
// Returns the wallpaper data or an error if the request fails or the wallpaper is not found.
// The id parameter should be the Wallhaven wallpaper ID (e.g., "6k3oox").
func (wh *WallhavenAPI) Wallpaper(id string) (Wallpaper, error) {
	urlBuilder := wh.urlbuilder.Clone()
	urlBuilder.Append(fmt.Sprintf("/w/%s", id))
	url := urlBuilder.Build()

	var wpQuery WallpaperQueryData
	if err := fetch.Json2Struct(url, &wpQuery); err != nil {
		return Wallpaper{}, err
	}
	return wpQuery.Data, nil
}

// Search creates a new query for searching wallpapers with a text query.
// The query parameter supports tags, usernames, and search terms.
// Returns a Query object that can be further configured with filters and executed.
// Use the returned Query's methods to add pagination or execute the search.
func (wh *WallhavenAPI) Search(query string) *Query {
	urlBuilder := wh.urlbuilder.Clone()
	urlBuilder.Append("/search")
	urlBuilder.SetString("q", query)
	return &Query{urlBuilder}
}

// TopList creates a new query for retrieving top-rated wallpapers.
// Automatically sets the sorting to toplist and applies any previously set filters.
// Returns a Query object that can be executed to get the most popular wallpapers.
// Use Range() to specify the time period (day, week, month, year) before executing.
func (wh *WallhavenAPI) TopList() *Query {
	wh.Sort(Toplist)
	urlBuilder := wh.urlbuilder.Clone()
	urlBuilder.Append("/search")
	return &Query{urlBuilder}
}

// Hot creates a new query for retrieving currently trending wallpapers.
// Automatically sets the sorting to hot and applies any previously set filters.
// Returns a Query object that can be executed to get wallpapers that are trending now.
func (wh *WallhavenAPI) Hot() *Query {
	wh.Sort(Hot)
	urlBuilder := wh.urlbuilder.Clone()
	urlBuilder.Append("/search")
	return &Query{urlBuilder}
}

// Page executes the query for a specific page number.
// The page parameter should be a positive integer starting from 1.
// Returns SearchQueryData containing wallpapers and metadata for the requested page,
// or an error if the request fails or the page number is invalid.
func (q *Query) Page(page int) (SearchQueryData, error) {
	cloned := q.URLBuilder.Clone()
	cloned.SetInt("page", page)
	return runQuery(cloned)
}

// Get executes the query and returns the first page of results.
// This is equivalent to calling Page(1) but more convenient for single-page requests.
// Returns SearchQueryData containing wallpapers and metadata for the first page,
// or an error if the request fails.
func (q *Query) Get() (SearchQueryData, error) {
	cloned := q.URLBuilder.Clone()
	return runQuery(cloned)
}

// Raw returns the query string to be run (excluding page numbers)
func (q *Query) Raw() string {
	cloned := q.URLBuilder.Clone()
	return cloned.Build()
}

// runQuery executes the HTTP request to the Wallhaven API and parses the JSON response.
// This is an internal helper function used by Page and Get methods.
// Returns SearchQueryData with the parsed response or an error if the request or parsing fails.
func runQuery(url *fetch.URLBuilder) (SearchQueryData, error) {
	urlString := url.Build()
	var searchQuery SearchQueryData
	if err := fetch.Json2Struct(urlString, &searchQuery); err != nil {
		return SearchQueryData{}, err
	}
	return searchQuery, nil
}
