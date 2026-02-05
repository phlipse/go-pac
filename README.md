# go-pac

go-pac provides a cross-platform way to retrieve the Proxy Auto-Configuration (PAC) URL from the operating system and evaluate PAC scripts for Go HTTP clients.

## Overview

- `GetPACURL()` reads the OS PAC URL and returns it as `*url.URL`.
- `NewPACProxy()` downloads and evaluates the PAC script, then returns a `*PACProxy`.
- `PACProxy.FindProxyStringForURL()` runs the PAC script for a target URL.
- `PACProxy.ProxyFunc()` returns a `http.Transport.Proxy` compatible function.

## Installation

```bash
go get github.com/phlipse/go-pac
```

## Basic usage

```go
package main

import (
	"log"
	"net/http"

	"github.com/phlipse/go-pac"
)

func main() {
	pacURL, err := pac.GetPACURL()
	if err != nil {
		log.Fatalf("get PAC URL: %v", err)
	}

	proxy, err := pac.NewPACProxy(pacURL, &pac.PACProxyConfig{})
	if err != nil {
		log.Fatalf("new PAC proxy: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: proxy.ProxyFunc(),
		},
	}

	resp, err := client.Get("http://example.com")
	if err != nil {
		log.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
}
```

## API

### GetPACURL

```go
func GetPACURL() (*url.URL, error)
```

Reads the PAC URL from the operating system:
- Windows: registry `AutoConfigURL`
- macOS: `scutil --proxy` output
- Linux (GNOME): `gsettings get org.gnome.system.proxy autoconfig-url`

Errors:
- `ErrPACURLNotFound` when no PAC URL is configured.
- `ErrPACURLEmpty` when a PAC key exists but is empty.

### NewPACProxy

```go
func NewPACProxy(pacURL *url.URL, config *PACProxyConfig) (*PACProxy, error)
```

Downloads the PAC script, evaluates it in the JavaScript runtime, and returns a `PACProxy`.

Errors:
- `ErrFetchPACScript` for HTTP/network errors or non-200 status.
- `ErrReadPACScript` for read errors.
- `ErrExecutePACScript` for script execution errors.
- `ErrPACScriptTooLarge` when the script exceeds `MaxScriptSize`.

### PACProxy

```go
type PACProxy struct {
	// opaque
}
```

Methods:

```go
func (p *PACProxy) FindProxyStringForURL(targetURL *url.URL) (ProxyString, error)
func (p *PACProxy) ProxyFunc() func(*http.Request) (*url.URL, error)
```

`FindProxyStringForURL` executes `FindProxyForURL(url, host)` inside the PAC script and returns the raw `ProxyString`.

`ProxyFunc` converts the `ProxyString` into a `*url.URL` suitable for `http.Transport.Proxy`.

Errors:
- `ErrEvaluatePAC` if `FindProxyForURL` is missing or execution fails.
- `ErrConvertResult` if the PAC result is not a string.
- `ErrPACScriptTimeout` when execution exceeds the configured timeout.

### PACProxyConfig

```go
type PACProxyConfig struct {
	Client           *http.Client
	MaxScriptSize    int64
	ScriptTimeout    time.Duration
	DNSLookupTimeout time.Duration
	HTTPTimeout      time.Duration
}
```

Defaults (used when values are zero):
- `HTTPTimeout`: 10s (only used when `Client == nil`)
- `ScriptTimeout`: 5s
- `DNSLookupTimeout`: 2s
- `MaxScriptSize`: 1 MiB

Disable a timeout or size limit by setting a negative value.

### ProxyString

```go
type ProxyString string

func (ps ProxyString) Parse() (*url.URL, error)
```

Parses the PAC result. Supported directives:
- `DIRECT`
- `PROXY host:port`
- `SOCKS host:port` (mapped to `socks5://`)

If multiple directives are returned (e.g. `PROXY a:1; PROXY b:2; DIRECT`), the first valid one is used. If none is valid, `ErrNoValidProxy` is returned.

## Notes

- PAC execution is serialized inside a single `PACProxy` instance (per script). Use multiple instances if you want to avoid lock contention.
- PAC scripts are executed with a JavaScript runtime (goja). The standard PAC helper functions are implemented.
