package pac

import "context"

// LogLevel represents a logging severity.
type LogLevel int

const (
	LogDebug LogLevel = iota
	LogInfo
	LogWarn
	LogError
)

// Logger is a minimal logging interface for this package.
// args should be key-value pairs (slog-style).
type Logger interface {
	Log(ctx context.Context, level LogLevel, msg string, args ...any)
}

// LoggerFunc adapts a function to the Logger interface.
type LoggerFunc func(ctx context.Context, level LogLevel, msg string, args ...any)

// Log calls the underlying function.
func (f LoggerFunc) Log(ctx context.Context, level LogLevel, msg string, args ...any) {
	f(ctx, level, msg, args...)
}

// LogHook can transform or suppress log entries.
// Return ok=false to drop the log entry.
type LogHook func(ctx context.Context, level LogLevel, msg string, args ...any) (outMsg string, outArgs []any, ok bool)

// RedactKeysHook redacts values for matching keys (case-insensitive).
func RedactKeysHook(keys ...string) LogHook {
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if k == "" {
			continue
		}
		set[toLower(k)] = struct{}{}
	}
	return func(_ context.Context, _ LogLevel, msg string, args ...any) (string, []any, bool) {
		if len(set) == 0 || len(args) == 0 {
			return msg, args, true
		}
		redacted := make([]any, len(args))
		copy(redacted, args)
		for i := 0; i+1 < len(redacted); i += 2 {
			key, ok := redacted[i].(string)
			if !ok {
				continue
			}
			if _, found := set[toLower(key)]; found {
				redacted[i+1] = "[REDACTED]"
			}
		}
		return msg, redacted, true
	}
}

func logf(ctx context.Context, l Logger, hook LogHook, level LogLevel, msg string, args ...any) {
	if l == nil {
		return
	}
	if hook != nil {
		var ok bool
		msg, args, ok = hook(ctx, level, msg, args...)
		if !ok {
			return
		}
	}
	l.Log(ctx, level, msg, args...)
}

func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] = b[i] - 'A' + 'a'
		}
	}
	return string(b)
}
