//go:build !unit && windows
// +build !unit,windows

package pac

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// retrievePACURL retrieves the PAC URL from the Windows registry.
func retrievePACURL() (string, error) {
	// Open the registry key where the PAC URL is stored
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	// Retrieve the PAC URL from the registry key
	pacURL, _, err := key.GetStringValue("AutoConfigURL")
	if err != nil {
		if err == registry.ErrNotExist {
			return "", ErrPACURLNotFound
		}
		return "", fmt.Errorf("failed to get registry value: %w", err)
	}

	// Check if the PAC URL is empty
	if pacURL == "" {
		return "", ErrPACURLEmpty
	}

	return pacURL, nil
}
