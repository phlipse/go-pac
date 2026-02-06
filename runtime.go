package pac

import (
	"context"
	"net"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// JSRuntime defines the interface for a JavaScript runtime
type JSRuntime interface {
	Set(name string, value interface{}) error
	RunString(script string) (goja.Value, error)
	DefinePACFunctions()
	Get(name string) goja.Value
	ToValue(value interface{}) goja.Value
	Interrupt(v interface{})
}

// GojaRuntime is an implementation of JSRuntime using goja
type GojaRuntime struct {
	*goja.Runtime
	dnsTimeout time.Duration
	defineErr  error
}

// NewGojaRuntime creates a new GojaRuntime instance
func NewGojaRuntime() *GojaRuntime {
	return &GojaRuntime{
		Runtime:    goja.New(),
		dnsTimeout: defaultDNSLookupTimeout,
	}
}

// SetDNSLookupTimeout sets the timeout for DNS lookups executed by PAC helpers.
func (r *GojaRuntime) SetDNSLookupTimeout(timeout time.Duration) {
	r.dnsTimeout = timeout
}

func (r *GojaRuntime) lookupHost(host string) ([]string, error) {
	if r.dnsTimeout <= 0 {
		return net.LookupHost(host)
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.dnsTimeout)
	defer cancel()
	return net.DefaultResolver.LookupHost(ctx, host)
}

func (r *GojaRuntime) resolveIP(host string) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return ip, nil
	}
	addrs, err := r.lookupHost(host)
	if err != nil || len(addrs) == 0 {
		return nil, err
	}
	return net.ParseIP(addrs[0]), nil
}

func (r *GojaRuntime) set(name string, value interface{}) {
	if r.defineErr != nil {
		return
	}
	if err := r.Set(name, value); err != nil {
		r.defineErr = err
	}
}

// DefinePACFunctions defines standard PAC functions in the JavaScript runtime
func (r *GojaRuntime) DefinePACFunctions() {
	r.set("isPlainHostName", func(call goja.FunctionCall) goja.Value {
		host := call.Argument(0).String()
		return r.ToValue(!strings.Contains(host, "."))
	})

	r.set("dnsDomainIs", func(call goja.FunctionCall) goja.Value {
		host := call.Argument(0).String()
		domain := call.Argument(1).String()
		return r.ToValue(strings.HasSuffix(host, domain))
	})

	r.set("localHostOrDomainIs", func(call goja.FunctionCall) goja.Value {
		host := call.Argument(0).String()
		hostdom := call.Argument(1).String()
		return r.ToValue(host == hostdom || strings.HasSuffix(hostdom, "."+host))
	})

	r.set("isResolvable", func(call goja.FunctionCall) goja.Value {
		host := call.Argument(0).String()
		_, err := r.lookupHost(host)
		return r.ToValue(err == nil)
	})

	r.set("isInNet", func(call goja.FunctionCall) goja.Value {
		host := call.Argument(0).String()
		pattern := call.Argument(1).String()
		mask := call.Argument(2).String()
		ip, err := r.resolveIP(host)
		if err != nil || ip == nil {
			return r.ToValue(false)
		}
		pat := net.ParseIP(pattern)
		m := net.ParseIP(mask)
		if ip == nil || pat == nil || m == nil {
			return r.ToValue(false)
		}
		if ip4 := ip.To4(); ip4 != nil {
			pat4 := pat.To4()
			m4 := m.To4()
			if pat4 == nil || m4 == nil {
				return r.ToValue(false)
			}
			for i := 0; i < 4; i++ {
				if (ip4[i] & m4[i]) != (pat4[i] & m4[i]) {
					return r.ToValue(false)
				}
			}
			return r.ToValue(true)
		}

		ip16 := ip.To16()
		pat16 := pat.To16()
		m16 := m.To16()
		if ip16 == nil || pat16 == nil || m16 == nil {
			return r.ToValue(false)
		}
		for i := 0; i < 16; i++ {
			if (ip16[i] & m16[i]) != (pat16[i] & m16[i]) {
				return r.ToValue(false)
			}
		}
		return r.ToValue(true)
	})

	r.set("dnsResolve", func(call goja.FunctionCall) goja.Value {
		host := call.Argument(0).String()
		addrs, err := r.lookupHost(host)
		if err != nil || len(addrs) == 0 {
			return r.ToValue("")
		}
		return r.ToValue(addrs[0])
	})

	r.set("myIpAddress", func(call goja.FunctionCall) goja.Value {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return r.ToValue("")
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return r.ToValue(ipnet.IP.String())
				}
			}
		}
		return r.ToValue("")
	})

	r.set("dnsDomainLevels", func(call goja.FunctionCall) goja.Value {
		host := call.Argument(0).String()
		return r.ToValue(strings.Count(host, "."))
	})

	r.set("shExpMatch", func(call goja.FunctionCall) goja.Value {
		str := call.Argument(0).String()
		pat := call.Argument(1).String()
		matched, _ := path.Match(pat, str)
		return r.ToValue(matched)
	})

	r.set("weekdayRange", func(call goja.FunctionCall) goja.Value {
		args, loc := splitArgsAndLocation(call.Arguments)
		if len(args) == 0 || len(args) > 2 {
			return r.ToValue(false)
		}
		wd1, ok := parseWeekdayArg(args[0])
		if !ok {
			return r.ToValue(false)
		}
		now := time.Now().In(loc).Weekday()
		if len(args) == 1 {
			return r.ToValue(now == wd1)
		}
		wd2, ok := parseWeekdayArg(args[1])
		if !ok {
			return r.ToValue(false)
		}
		if wd1 <= wd2 {
			return r.ToValue(now >= wd1 && now <= wd2)
		}
		return r.ToValue(now >= wd1 || now <= wd2)
	})

	r.set("dateRange", func(call goja.FunctionCall) goja.Value {
		args, loc := splitArgsAndLocation(call.Arguments)
		if len(args) == 0 {
			return r.ToValue(false)
		}
		now := time.Now().In(loc)
		return r.ToValue(dateRangeMatches(args, now, loc))
	})

	r.set("timeRange", func(call goja.FunctionCall) goja.Value {
		args, loc := splitArgsAndLocation(call.Arguments)
		if len(args) == 0 {
			return r.ToValue(false)
		}
		now := time.Now().In(loc)
		return r.ToValue(timeRangeMatches(args, now))
	})
}

var weekdayNames = map[string]time.Weekday{
	"SUN": time.Sunday,
	"MON": time.Monday,
	"TUE": time.Tuesday,
	"WED": time.Wednesday,
	"THU": time.Thursday,
	"FRI": time.Friday,
	"SAT": time.Saturday,
}

var monthNames = map[string]time.Month{
	"JAN": time.January,
	"FEB": time.February,
	"MAR": time.March,
	"APR": time.April,
	"MAY": time.May,
	"JUN": time.June,
	"JUL": time.July,
	"AUG": time.August,
	"SEP": time.September,
	"OCT": time.October,
	"NOV": time.November,
	"DEC": time.December,
}

type dateArgKind int

const (
	dateArgDay dateArgKind = iota
	dateArgMonth
	dateArgYear
)

type dateArg struct {
	kind  dateArgKind
	value int
}

func splitArgsAndLocation(args []goja.Value) ([]goja.Value, *time.Location) {
	loc := time.Local
	if len(args) == 0 {
		return args, loc
	}
	if isGMTArg(args[len(args)-1]) {
		return args[:len(args)-1], time.UTC
	}
	return args, loc
}

func isGMTArg(v goja.Value) bool {
	if v == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(v.String()), "GMT")
}

func parseIntArg(v goja.Value) (int, bool) {
	if v == nil {
		return 0, false
	}

	switch val := v.Export().(type) {
	case int:
		return val, true
	case int32:
		return int(val), true
	case int64:
		return int(val), true
	case float32:
		f := float64(val)
		i := int(f)
		if float64(i) != f {
			return 0, false
		}
		return i, true
	case float64:
		i := int(val)
		if float64(i) != val {
			return 0, false
		}
		return i, true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		s := strings.TrimSpace(v.String())
		i, err := strconv.Atoi(s)
		if err != nil {
			return 0, false
		}
		return i, true
	}
}

func parseWeekdayArg(v goja.Value) (time.Weekday, bool) {
	if v == nil {
		return 0, false
	}
	if s, ok := v.Export().(string); ok {
		s = strings.ToUpper(strings.TrimSpace(s))
		if len(s) >= 3 {
			s = s[:3]
		}
		if day, ok := weekdayNames[s]; ok {
			return day, true
		}
	}
	if n, ok := parseIntArg(v); ok && n >= 0 && n <= 6 {
		return time.Weekday(n), true
	}
	return 0, false
}

func parseDateArg(v goja.Value) (dateArg, bool) {
	if v == nil {
		return dateArg{}, false
	}
	if s, ok := v.Export().(string); ok {
		s = strings.ToUpper(strings.TrimSpace(s))
		if len(s) >= 3 {
			s = s[:3]
		}
		if month, ok := monthNames[s]; ok {
			return dateArg{kind: dateArgMonth, value: int(month)}, true
		}
	}
	n, ok := parseIntArg(v)
	if !ok || n <= 0 {
		return dateArg{}, false
	}
	if n <= 31 {
		return dateArg{kind: dateArgDay, value: n}, true
	}
	return dateArg{kind: dateArgYear, value: normalizeYear(n)}, true
}

func normalizeYear(year int) int {
	if year >= 0 && year < 100 {
		return 1900 + year
	}
	return year
}

func dateRangeMatches(args []goja.Value, now time.Time, loc *time.Location) bool {
	parsed := make([]dateArg, 0, len(args))
	for _, arg := range args {
		value, ok := parseDateArg(arg)
		if !ok {
			return false
		}
		parsed = append(parsed, value)
	}

	current := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	switch len(parsed) {
	case 1:
		arg := parsed[0]
		switch arg.kind {
		case dateArgDay:
			return now.Day() == arg.value
		case dateArgMonth:
			return int(now.Month()) == arg.value
		case dateArgYear:
			return now.Year() == arg.value
		}
	case 2:
		first, second := parsed[0], parsed[1]
		switch {
		case first.kind == dateArgDay && second.kind == dateArgDay:
			return dayRangeMatches(now.Day(), first.value, second.value)
		case first.kind == dateArgMonth && second.kind == dateArgMonth:
			return monthRangeMatches(int(now.Month()), first.value, second.value)
		case first.kind == dateArgYear && second.kind == dateArgYear:
			return yearRangeMatches(now.Year(), first.value, second.value)
		case first.kind == dateArgDay && second.kind == dateArgMonth:
			return now.Day() == first.value && int(now.Month()) == second.value
		case first.kind == dateArgMonth && second.kind == dateArgYear:
			return int(now.Month()) == first.value && now.Year() == second.value
		}
	case 3:
		first, second, third := parsed[0], parsed[1], parsed[2]
		if first.kind == dateArgDay && second.kind == dateArgMonth && third.kind == dateArgYear {
			return now.Day() == first.value && int(now.Month()) == second.value && now.Year() == third.value
		}
	case 4:
		first, second, third, fourth := parsed[0], parsed[1], parsed[2], parsed[3]
		if first.kind == dateArgDay && second.kind == dateArgMonth && third.kind == dateArgDay && fourth.kind == dateArgMonth {
			start, ok := makeDate(now.Year(), time.Month(second.value), first.value, loc)
			if !ok {
				return false
			}
			end, ok := makeDate(now.Year(), time.Month(fourth.value), third.value, loc)
			if !ok {
				return false
			}
			return dateRangeCompare(current, start, end, true)
		}
		if first.kind == dateArgMonth && second.kind == dateArgYear && third.kind == dateArgMonth && fourth.kind == dateArgYear {
			start, ok := makeDate(second.value, time.Month(first.value), 1, loc)
			if !ok {
				return false
			}
			endDay := lastDayOfMonth(fourth.value, time.Month(third.value), loc)
			end, ok := makeDate(fourth.value, time.Month(third.value), endDay, loc)
			if !ok {
				return false
			}
			return dateRangeCompare(current, start, end, false)
		}
	case 5:
		first, second, third, fourth, fifth := parsed[0], parsed[1], parsed[2], parsed[3], parsed[4]
		if first.kind == dateArgDay && second.kind == dateArgMonth && third.kind == dateArgYear &&
			fourth.kind == dateArgDay && fifth.kind == dateArgMonth {
			start, ok := makeDate(third.value, time.Month(second.value), first.value, loc)
			if !ok {
				return false
			}
			end, ok := makeDate(third.value, time.Month(fifth.value), fourth.value, loc)
			if !ok {
				return false
			}
			return dateRangeCompare(current, start, end, true)
		}
	case 6:
		first, second, third, fourth, fifth, sixth := parsed[0], parsed[1], parsed[2], parsed[3], parsed[4], parsed[5]
		if first.kind == dateArgDay && second.kind == dateArgMonth && third.kind == dateArgYear &&
			fourth.kind == dateArgDay && fifth.kind == dateArgMonth && sixth.kind == dateArgYear {
			start, ok := makeDate(third.value, time.Month(second.value), first.value, loc)
			if !ok {
				return false
			}
			end, ok := makeDate(sixth.value, time.Month(fifth.value), fourth.value, loc)
			if !ok {
				return false
			}
			return dateRangeCompare(current, start, end, false)
		}
	}

	return false
}

func makeDate(year int, month time.Month, day int, loc *time.Location) (time.Time, bool) {
	if day <= 0 {
		return time.Time{}, false
	}
	t := time.Date(year, month, day, 0, 0, 0, 0, loc)
	if t.Year() != year || t.Month() != month || t.Day() != day {
		return time.Time{}, false
	}
	return t, true
}

func lastDayOfMonth(year int, month time.Month, loc *time.Location) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
}

func dateRangeCompare(current, start, end time.Time, allowWrap bool) bool {
	if !allowWrap || !end.Before(start) {
		return !current.Before(start) && !current.After(end)
	}
	if current.Before(start) {
		start = start.AddDate(-1, 0, 0)
	} else {
		end = end.AddDate(1, 0, 0)
	}
	return !current.Before(start) && !current.After(end)
}

func dayRangeMatches(current, start, end int) bool {
	if start <= end {
		return current >= start && current <= end
	}
	return current >= start || current <= end
}

func monthRangeMatches(current, start, end int) bool {
	if start <= end {
		return current >= start && current <= end
	}
	return current >= start || current <= end
}

func yearRangeMatches(current, start, end int) bool {
	if start <= end {
		return current >= start && current <= end
	}
	return current >= start || current <= end
}

func timeRangeMatches(args []goja.Value, now time.Time) bool {
	if len(args) == 0 {
		return false
	}

	values := make([]int, 0, len(args))
	for _, arg := range args {
		v, ok := parseIntArg(arg)
		if !ok {
			return false
		}
		values = append(values, v)
	}

	nowSeconds := now.Hour()*3600 + now.Minute()*60 + now.Second()

	switch len(values) {
	case 1:
		h := values[0]
		if !validHour(h) {
			return false
		}
		return now.Hour() == h
	case 2:
		h1, h2 := values[0], values[1]
		if !validHour(h1) || !validHour(h2) {
			return false
		}
		start := h1 * 3600
		end := h2*3600 + 3599
		return timeRangeCompare(nowSeconds, start, end)
	case 3:
		h, m, s := values[0], values[1], values[2]
		if !validTime(h, m, s) {
			return false
		}
		return now.Hour() == h && now.Minute() == m && now.Second() == s
	case 4:
		h1, m1, h2, m2 := values[0], values[1], values[2], values[3]
		if !validTime(h1, m1, 0) || !validTime(h2, m2, 0) {
			return false
		}
		start := h1*3600 + m1*60
		end := h2*3600 + m2*60 + 59
		return timeRangeCompare(nowSeconds, start, end)
	case 6:
		h1, m1, s1 := values[0], values[1], values[2]
		h2, m2, s2 := values[3], values[4], values[5]
		if !validTime(h1, m1, s1) || !validTime(h2, m2, s2) {
			return false
		}
		start := h1*3600 + m1*60 + s1
		end := h2*3600 + m2*60 + s2
		return timeRangeCompare(nowSeconds, start, end)
	default:
		return false
	}
}

func validHour(hour int) bool {
	return hour >= 0 && hour <= 23
}

func validTime(hour, minute, second int) bool {
	return validHour(hour) && minute >= 0 && minute <= 59 && second >= 0 && second <= 59
}

func timeRangeCompare(currentSeconds, startSeconds, endSeconds int) bool {
	if startSeconds <= endSeconds {
		return currentSeconds >= startSeconds && currentSeconds <= endSeconds
	}
	return currentSeconds >= startSeconds || currentSeconds <= endSeconds
}
