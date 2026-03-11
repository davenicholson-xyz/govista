package fetch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func Json(url string) ([]byte, error) {

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("Error:404")
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("401 - Unauthorized")
	}

	// fmt.Println(string(body))
	return body, nil
}

func Json2Struct[T any](url string, obj *T) error {

	resp, err := Json(url)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(resp, obj); err != nil {
		return err
	}

	return nil

}
