//go:build !unit && linux
// +build !unit,linux

package pac

import (
	"fmt"
	"os/exec"
	"strings"
)

// retrievePACURL retrieves the PAC URL from GNOME settings on Linux using the gsettings command.
// Note: This function currently only supports GNOME.
func retrievePACURL() (string, error) {
	// Run the gsettings command to get the autoconfig URL
	cmd := exec.Command("gsettings", "get", "org.gnome.system.proxy", "autoconfig-url")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run gsettings command: %w", err)
	}

	// Trim and clean up the output
	pacURL := strings.TrimSpace(string(out))
	pacURL = strings.Trim(pacURL, "'") // Remove single quotes if present

	// Check if the PAC URL is empty
	if pacURL == "" {
		return "", ErrPACURLNotFound
	}

	return pacURL, nil
}
