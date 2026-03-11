package wallhavenapi

import (
	"fmt"

	"github.com/davenicholson-xyz/go-wallhaven/wallhavenapi/fetch"
)

// Tag retrieves detailed information about a specific tag by its ID.
// The id parameter should be the numeric tag ID from Wallhaven (e.g., 1 for "anime").
// Returns Tag data including the tag name, category, purity level, and creation date,
// or an error if the request fails or the tag is not found.
func (wh *WallhavenAPI) Tag(id int) (Tag, error) {
	wh.urlbuilder.Append(fmt.Sprintf("/tag/%d", id))
	url := wh.urlbuilder.Build()

	var tagQuery TagData
	if err := fetch.Json2Struct(url, &tagQuery); err != nil {
		return Tag{}, err
	}

	return tagQuery.Data, nil
}
