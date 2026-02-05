package pac

import (
	"errors"
	"fmt"
	"net/url"
)

// Custom error types
var (
	ErrPACURLNotFound = errors.New("PAC URL not found")
	ErrPACURLEmpty    = errors.New("PAC URL is empty")
)

// GetPACURL retrieves the PAC URL from the operating system and returns it as a sanitized *url.URL.
func GetPACURL() (*url.URL, error) {
	// Retrieve the PAC URL as a string from the operating system
	pacURL, err := retrievePACURL()
	if err != nil {
		return nil, fmt.Errorf("failed to get PAC URL: %w", err)
	}

	// Parse the PAC URL string into a *url.URL object
	parsedURL, err := url.Parse(pacURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PAC URL: %w", err)
	}

	return parsedURL, nil
}
