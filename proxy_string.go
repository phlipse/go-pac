package pac

import (
	"errors"
	"net/url"
	"strings"
)

// Custom error types
var (
	ErrNoValidProxy = errors.New("no valid proxy found")
)

// ProxyString represents a proxy string
type ProxyString string

// Parse parses the proxy string and returns the appropriate proxy URL.
// If multiple proxies are contained in ProxyString, first one is returned.
func (ps ProxyString) Parse() (*url.URL, error) {
	proxies := strings.Split(string(ps), ";")
	for _, proxy := range proxies {
		proxy = strings.TrimSpace(proxy)
		if strings.HasPrefix(proxy, "DIRECT") {
			return nil, nil
		}
		if strings.HasPrefix(proxy, "PROXY") {
			proxyURL, err := url.Parse("http://" + strings.TrimPrefix(proxy, "PROXY "))
			if err != nil {
				return nil, err
			}
			return proxyURL, nil
		}
		if strings.HasPrefix(proxy, "SOCKS") {
			proxyURL, err := url.Parse("socks5://" + strings.TrimPrefix(proxy, "SOCKS "))
			if err != nil {
				return nil, err
			}
			return proxyURL, nil
		}
	}

	return nil, ErrNoValidProxy
}
