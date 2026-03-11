package wallhavenapi

import (
	"fmt"

	"github.com/davenicholson-xyz/go-wallhaven/wallhavenapi/fetch"
)

// UserSettings retrieves the current user's account settings and preferences.
// This function requires an API key to be set using ApiKey() before calling.
// Returns UserSettings containing account preferences, avatar, and profile information,
// or an error if no API key is provided or the request fails.
func (wh *WallhavenAPI) UserSettings() (UserSettings, error) {
	key := wh.urlbuilder.Has("apikey")
	if !key {
		return UserSettings{}, fmt.Errorf("API key required to fetch user settings")
	}

	urlBuilder := wh.urlbuilder.Clone()
	urlBuilder.Append("/settings")
	url := urlBuilder.Build()

	var userQuery UserSettingsData
	if err := fetch.Json2Struct(url, &userQuery); err != nil {
		return UserSettings{}, err
	}

	return userQuery.Data, nil
}

// MyCollections retrieves all collections belonging to the authenticated user.
// This function requires an API key to be set using ApiKey() before calling.
// Returns a slice of Collection objects containing the user's personal collections,
// or an error if no API key is provided or the request fails.
// Only collections owned by the authenticated user are returned.
func (wh *WallhavenAPI) MyCollections() ([]Collection, error) {
	key := wh.urlbuilder.Has("apikey")
	if !key {
		return []Collection{}, fmt.Errorf("API key required to fetch your collections")
	}

	urlBuilder := wh.urlbuilder.Clone()
	urlBuilder.Append("/collections")
	url := urlBuilder.Build()

	var collectionsQuery CollectionData
	if err := fetch.Json2Struct(url, &collectionsQuery); err != nil {
		return []Collection{}, err
	}

	return collectionsQuery.Data, nil
}

// Collections retrieves all public collections for a specific user by username.
// The username parameter should be the Wallhaven username (not display name).
// Returns a slice of Collection objects containing the user's public collections,
// or an error if the user is not found or the request fails.
// Private collections are not included in the results unless you have appropriate access.
func (wh *WallhavenAPI) Collections(username string) ([]Collection, error) {
	urlBuilder := wh.urlbuilder.Clone()
	urlBuilder.Append(fmt.Sprintf("/collections/%s", username))
	url := urlBuilder.Build()

	var collectionsQuery CollectionData
	if err := fetch.Json2Struct(url, &collectionsQuery); err != nil {
		return []Collection{}, err
	}

	return collectionsQuery.Data, nil
}

// Collection creates a new query for returning wallpapers from a collection.
// Use the returned Query's methods to fetch the first page via Get() or Page(x)
func (wh *WallhavenAPI) Collection(username string, id int) *Query {
	urlBuilder := wh.urlbuilder.Clone()
	urlBuilder.Append(fmt.Sprintf("/collections/%s/%d", username, id))
	return &Query{urlBuilder}
}
