//go:build unit
// +build unit

package pac

import (
	"strings"
	"sync"
)

var (
	testPACURL   string
	testPACURLMu sync.RWMutex
)

// SetTestPACURL overrides the OS PAC URL lookup for tests.
// Pass an empty string to reset to "not found".
func SetTestPACURL(pacURL string) {
	testPACURLMu.Lock()
	testPACURL = pacURL
	testPACURLMu.Unlock()
}

func retrievePACURL() (string, error) {
	testPACURLMu.RLock()
	pacURL := testPACURL
	testPACURLMu.RUnlock()

	if strings.TrimSpace(pacURL) == "" {
		return "", ErrPACURLNotFound
	}
	return pacURL, nil
}
