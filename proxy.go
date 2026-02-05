package pac

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// Custom error types
var (
	ErrFetchPACScript    = errors.New("failed to fetch PAC script")
	ErrReadPACScript     = errors.New("failed to read PAC script")
	ErrExecutePACScript  = errors.New("failed to execute PAC script")
	ErrEvaluatePAC       = errors.New("error evaluating PAC script")
	ErrConvertResult     = errors.New("error converting result to string")
	ErrPACScriptTimeout  = errors.New("PAC script execution timed out")
	ErrPACScriptTooLarge = errors.New("PAC script exceeds maximum size")
)

const (
	defaultHTTPTimeout      = 10 * time.Second
	defaultScriptTimeout    = 5 * time.Second
	defaultDNSLookupTimeout = 2 * time.Second
	defaultMaxScriptSize    = 1 << 20 // 1 MiB
)

// PACProxy holds the PAC script, the JavaScript VM and custom HTTP client
type PACProxy struct {
	script string
	vm     JSRuntime
	mu     sync.Mutex
	client *http.Client

	scriptTimeout time.Duration
}

// PACProxyConfig holds configuration options for Proxy
type PACProxyConfig struct {
	Client           *http.Client
	MaxScriptSize    int64
	ScriptTimeout    time.Duration
	DNSLookupTimeout time.Duration
	HTTPTimeout      time.Duration
}

// NewPACProxy creates a new Proxy instance with the given configuration
func NewPACProxy(pacURL *url.URL, config *PACProxyConfig) (*PACProxy, error) {
	cfg := normalizePACProxyConfig(config)
	client := cfg.Client

	// Fetch the PAC script from the provided URL
	resp, err := client.Get(pacURL.String())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetchPACScript, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status code %d", ErrFetchPACScript, resp.StatusCode)
	}

	if cfg.MaxScriptSize > 0 && resp.ContentLength > cfg.MaxScriptSize {
		return nil, ErrPACScriptTooLarge
	}

	// Read the PAC script with size limits
	script, err := readPACScript(resp.Body, cfg.MaxScriptSize)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReadPACScript, err)
	}

	// Create a new JavaScript runtime and define standard PAC functions
	vm := NewGojaRuntime()
	vm.SetDNSLookupTimeout(cfg.DNSLookupTimeout)
	vm.DefinePACFunctions()

	// Execute the PAC script in the JavaScript runtime
	err = runWithTimeout(vm, cfg.ScriptTimeout, func() error {
		_, runErr := vm.RunString(string(script))
		return runErr
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrExecutePACScript, err)
	}

	return &PACProxy{
		script:        string(script),
		vm:            vm,
		client:        client,
		scriptTimeout: cfg.ScriptTimeout,
	}, nil
}

// FindProxyForURL evaluates the PAC script to find the proxy for a given URL
func (p *PACProxy) FindProxyStringForURL(targetURL *url.URL) (ProxyString, error) {
	result, err := p.evalWithTimeout(func() (goja.Value, error) {
		// Call the JavaScript function FindProxyForURL with the URL and host as parameters
		fn, ok := goja.AssertFunction(p.vm.Get("FindProxyForURL"))
		if !ok {
			return nil, ErrEvaluatePAC
		}

		value, callErr := fn(goja.Undefined(), p.vm.ToValue(targetURL.String()), p.vm.ToValue(targetURL.Host))
		if callErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrEvaluatePAC, callErr)
		}

		return value, nil
	})
	if err != nil {
		return "", err
	}

	proxyStr, ok := result.Export().(string)
	if !ok {
		return "", ErrConvertResult
	}

	return ProxyString(proxyStr), nil
}

// PACProxyFunc returns a function that can be used as the Proxy parameter in http.Transport
func (p *PACProxy) ProxyFunc() func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		proxyStr, err := p.FindProxyStringForURL(req.URL)
		if err != nil {
			return nil, err
		}

		ps := ProxyString(proxyStr)
		return ps.Parse()
	}
}

type pacEvalResult struct {
	value goja.Value
	err   error
}

func (p *PACProxy) evalWithTimeout(fn func() (goja.Value, error)) (goja.Value, error) {
	if p.scriptTimeout <= 0 {
		p.mu.Lock()
		defer p.mu.Unlock()
		return fn()
	}

	resultCh := make(chan pacEvalResult, 1)
	started := make(chan struct{})

	go func() {
		p.mu.Lock()
		close(started)
		value, err := fn()
		p.mu.Unlock()
		resultCh <- pacEvalResult{value: value, err: err}
	}()

	<-started
	timer := time.NewTimer(p.scriptTimeout)
	defer timer.Stop()

	select {
	case res := <-resultCh:
		return res.value, normalizePACError(res.err)
	case <-timer.C:
		select {
		case res := <-resultCh:
			return res.value, normalizePACError(res.err)
		default:
		}
		p.vm.Interrupt(ErrPACScriptTimeout)
		res := <-resultCh
		if res.err == nil {
			res.err = ErrPACScriptTimeout
		}
		return res.value, normalizePACError(res.err)
	}
}

func normalizePACProxyConfig(config *PACProxyConfig) PACProxyConfig {
	cfg := PACProxyConfig{}
	if config != nil {
		cfg = *config
	}

	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = defaultHTTPTimeout
	} else if cfg.HTTPTimeout < 0 {
		cfg.HTTPTimeout = 0
	}

	if cfg.ScriptTimeout == 0 {
		cfg.ScriptTimeout = defaultScriptTimeout
	} else if cfg.ScriptTimeout < 0 {
		cfg.ScriptTimeout = 0
	}

	if cfg.DNSLookupTimeout == 0 {
		cfg.DNSLookupTimeout = defaultDNSLookupTimeout
	} else if cfg.DNSLookupTimeout < 0 {
		cfg.DNSLookupTimeout = 0
	}

	if cfg.MaxScriptSize == 0 {
		cfg.MaxScriptSize = defaultMaxScriptSize
	} else if cfg.MaxScriptSize < 0 {
		cfg.MaxScriptSize = 0
	}

	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: cfg.HTTPTimeout}
	}

	return cfg
}

func readPACScript(r io.Reader, maxSize int64) ([]byte, error) {
	if maxSize <= 0 {
		return io.ReadAll(r)
	}

	limited := io.LimitReader(r, maxSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, ErrPACScriptTooLarge
	}
	return data, nil
}

func runWithTimeout(vm JSRuntime, timeout time.Duration, fn func() error) error {
	if timeout <= 0 {
		return normalizePACError(fn())
	}

	resultCh := make(chan error, 1)
	go func() {
		resultCh <- fn()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-resultCh:
		return normalizePACError(err)
	case <-timer.C:
		select {
		case err := <-resultCh:
			return normalizePACError(err)
		default:
		}
		vm.Interrupt(ErrPACScriptTimeout)
		err := <-resultCh
		if err == nil {
			return ErrPACScriptTimeout
		}
		return normalizePACError(err)
	}
}

func normalizePACError(err error) error {
	if err == nil {
		return nil
	}
	var interrupted *goja.InterruptedError
	if errors.As(err, &interrupted) {
		if interrupted.Value() == ErrPACScriptTimeout {
			return ErrPACScriptTimeout
		}
	}
	return err
}
