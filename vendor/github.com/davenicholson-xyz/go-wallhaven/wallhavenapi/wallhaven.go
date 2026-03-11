// Package wallhavenapi provides an API wrapper for wallhaven.cc
//
// This package allows you to search for wallpapers, retrieve wallpaper details,
// access user collections, and manage API interactions with the Wallhaven service.
// It supports both authenticated and unauthenticated requests, with API key
// authentication required for accessing NSFW content and user-specific data.
//
// Basic usage:
//
//	client := wallhavenapi.New()
//	results, err := client.Search("nature").Get()
//
// With API key for authenticated requests:
//
//	client := wallhavenapi.NewWithAPIKey("your-api-key")
//	settings, err := client.UserSettings()
package wallhavenapi

import (
	"github.com/davenicholson-xyz/go-wallhaven/wallhavenapi/fetch"
)

// WallhavenAPI represents the main API client for interacting with Wallhaven.
// It maintains URL building state and configuration for making API requests.
// Use New() or NewWithApiKey() to create a new instance.
type WallhavenAPI struct {
	urlbuilder *fetch.URLBuilder
}

// New creates a new WallhavenAPI client for unauthenticated requests.
// This client can search for wallpapers and access public data, but cannot
// access NSFW content or user-specific endpoints that require authentication.
// Use ApiKey() method to add authentication later, or use NewWithApiKey() instead.
func New() *WallhavenAPI {
	return &WallhavenAPI{
		urlbuilder: fetch.NewURL("https://wallhaven.cc/api/v1"),
	}
}

// NewWithApiKey creates a new WallhavenAPI client with an API key for authenticated requests.
// The apikey parameter should be your personal Wallhaven API key obtained from your account settings.
// This client can access all endpoints including NSFW content and user-specific data.
// Returns a configured WallhavenAPI instance ready for authenticated requests.
func NewWithAPIKey(apikey string) *WallhavenAPI {
	urlbuilder := fetch.NewURL("https://wallhaven.cc/api/v1")
	urlbuilder.SetString("apikey", apikey)
	return &WallhavenAPI{
		urlbuilder: urlbuilder,
	}
}
