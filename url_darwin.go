//go:build !unit && darwin
// +build !unit,darwin

package pac

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// retrievePACURL retrieves the PAC URL from macOS using the scutil command.
func retrievePACURL() (string, error) {
	// Run the scutil command to get proxy settings
	cmd := exec.Command("scutil", "--proxy")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run scutil command: %w", err)
	}

	// Parse the output to find the ProxyAutoConfigURL
	output := out.String()
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "ProxyAutoConfigURL") {
			parts := strings.Split(line, ": ")
			if len(parts) == 2 {
				pacURL := strings.TrimSpace(parts[1])
				// Check if the PAC URL is empty
				if pacURL == "" {
					return "", ErrPACURLEmpty
				}
				return pacURL, nil
			}
		}
	}

	return "", ErrPACURLNotFound
}
