package pac_test

import (
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/phlipse/go-pac"
)

// TestGetURL tests the GetURL function to ensure it correctly retrieves the PAC URL.
func TestGetPACURL(t *testing.T) {
	// Call GetURL to retrieve the PAC URL
	pacURL, err := pac.GetPACURL()
	if err != nil {
		t.Fatalf("Error retrieving PAC URL: %v", err)
	}

	// Verify that the PAC URL is not nil
	if pacURL == nil {
		t.Fatalf("Expected PAC URL to be non-nil")
	}

	t.Logf("PAC URL: %s\n", pacURL.String())
}

// TestFindProxyForURL tests the FindProxyForURL function to ensure it correctly evaluates the PAC script.
func TestFindProxyStringForURL(t *testing.T) {
	// Call GetURL to retrieve the PAC URL
	pacURL, err := pac.GetPACURL()
	if err != nil {
		t.Fatalf("Error retrieving PAC URL: %v", err)
	}

	// Create a ProxyConfig with a custom HTTP client with timeout
	config := &pac.PACProxyConfig{
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	// Call NewProxy to create a new Proxy instance
	proxy, err := pac.NewPACProxy(pacURL, config)
	if err != nil {
		t.Fatalf("Error creating Proxy instance: %v", err)
	}

	// Define a sample target URL for testing
	targetURL, err := url.Parse("http://example.com")
	if err != nil {
		t.Fatalf("Failed to parse target URL: %v", err)
	}

	// Call FindProxyForURL to evaluate the PAC script for the target URL
	proxyStr, err := proxy.FindProxyStringForURL(targetURL)
	if err != nil {
		t.Fatalf("Error finding proxy for URL: %v", err)
	}

	// Verify that the proxy string is not empty
	if proxyStr == "" {
		t.Fatalf("Expected proxy string to be non-empty")
	}

	t.Logf("proxy string: %s\n", proxyStr)
}

// TestParse tests the Parse method of the ProxyString type to ensure it correctly parses proxy strings.
func TestParse(t *testing.T) {
	tests := []struct {
		proxyStr    pac.ProxyString
		expectedURL string
		expectedErr error
	}{
		{
			proxyStr:    "DIRECT",
			expectedURL: "",
			expectedErr: nil,
		},
		{
			proxyStr:    "PROXY proxy.example.com:8080",
			expectedURL: "http://proxy.example.com:8080",
			expectedErr: nil,
		},
		{
			proxyStr:    "PROXY proxy.example.com:8080; PROXY proxy.example.com:8081; PROXY proxy.example.com:8082; PROXY proxy.example.com:8083;",
			expectedURL: "http://proxy.example.com:8080",
			expectedErr: nil,
		},
		{
			proxyStr:    "SOCKS socks.example.com:1080",
			expectedURL: "socks5://socks.example.com:1080",
			expectedErr: nil,
		},
		{
			proxyStr:    "INVALID proxy.example.com:8080",
			expectedURL: "",
			expectedErr: pac.ErrNoValidProxy,
		},
	}

	for _, test := range tests {
		t.Run(string(test.proxyStr), func(t *testing.T) {
			proxyURL, err := test.proxyStr.Parse()
			if err != test.expectedErr {
				t.Fatalf("Expected error %v, got %v", test.expectedErr, err)
			}

			if proxyURL != nil && proxyURL.String() != test.expectedURL {
				t.Fatalf("Expected URL %s, got %s", test.expectedURL, proxyURL.String())
			}

			if proxyURL == nil && test.expectedURL != "" {
				t.Fatalf("Expected URL %s, got nil", test.expectedURL)
			}
		})
	}
}

// TestNewProxy tests the NewProxy function to ensure it correctly creates a Proxy instance.
func TestNewPACProxy(t *testing.T) {
	// Retrieve the PAC URL from the operating system
	pacURL, err := pac.GetPACURL()
	if err != nil {
		t.Fatalf("Error retrieving PAC URL: %v\n", err)
	}
	t.Logf("PAC URL: %s\n", pacURL.String())

	// Create a ProxyConfig with a custom HTTP client with timeout
	config := &pac.PACProxyConfig{
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	proxy, err := pac.NewPACProxy(pacURL, config)
	if err != nil {
		t.Fatalf("Error creating PAC proxy: %v\n", err)
	}

	// Configure an HTTP client to use the PAC proxy
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: proxy.ProxyFunc(),
		},
		Timeout: 10 * time.Second,
	}

	// Make a request using the configured HTTP client
	resp, err := client.Get("http://example.com")
	if err != nil {
		t.Fatalf("Error making request: %v\n", err)
	}
	defer resp.Body.Close()

	// Read and log the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Error reading response body: %v\n", err)
	}

	t.Logf("Response: %s\n", body)
}
