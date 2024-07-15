/*
Package tinter implements a zero-dependency [slog.Handler] that writes tinted
(colorized) logs. The output format is inspired by the [zerolog.ConsoleWriter]
and [slog.TextHandler].

The output format can be customized using [Options], which is a drop-in
replacement for [slog.HandlerOptions].

# Customize Attributes

Options.ReplaceAttr can be used to alter or drop attributes. If set, it is
called on each non-group attribute before it is logged.
See [slog.HandlerOptions] for details.

	w := os.Stderr
	logger := slog.New(
		tinter.NewHandler(w, &tinter.Options{
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey && len(groups) == 0 {
					return slog.Attr{}
				}
				return a
			},
		}),
	)

# Automatically Enable Colors

Colors are enabled by default and can be disabled using the Options.NoColor
attribute. To automatically enable colors based on the terminal capabilities,
use e.g. the [go-isatty] package.

	w := os.Stderr
	logger := slog.New(
		tinter.NewHandler(w, &tinter.Options{
			NoColor: !isatty.IsTerminal(w.Fd()),
		}),
	)

# Windows Support

Color support on Windows can be added by using e.g. the [go-colorable] package.

	w := os.Stderr
	logger := slog.New(
		tinter.NewHandler(colorable.NewColorable(w), nil),
	)

[zerolog.ConsoleWriter]: https://pkg.go.dev/github.com/rs/zerolog#ConsoleWriter
[go-isatty]: https://pkg.go.dev/github.com/mattn/go-isatty
[go-colorable]: https://pkg.go.dev/github.com/mattn/go-colorable
*/
package tinter

import (
	"context"
	"encoding"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unicode"
)

// ANSI modes
const (
	ansiReset              = "\033[0m"
	ansiFaint              = "\033[2m"
	ansiBrightMagentaFaint = "\033[95;2m"
	ansiResetFaint         = "\033[22m"
	ansiBrightRed          = "\033[91m"
	ansiBrightRedFaint     = "\033[91;2m"
	ansiBrightGreen        = "\033[92m"
	ansiBrightYellow       = "\033[93m"
)

var (
	defaultLevel      = slog.LevelInfo
	defaultTimeFormat = time.StampMilli
)

// Options for a slog.Handler that writes tinted logs. A zero Options consists
// entirely of default values.
//
// Options can be used as a drop-in replacement for [slog.HandlerOptions].
type Options struct {
	// Enable source code location (Default: false)
	AddSource bool

	// Minimum level to log (Default: slog.LevelInfo)
	Level slog.Leveler

	// ReplaceAttr is called to rewrite each non-group attribute before it is logged.
	// See https://pkg.go.dev/log/slog#HandlerOptions for details.
	ReplaceAttr func(groups []string, attr slog.Attr) slog.Attr

	// Time format (Default: time.StampMilli)
	TimeFormat string

	// Disable color (Default: false)
	NoColor bool
}

// NewHandler creates a [slog.Handler] that writes tinted logs to Writer w,
// using the default options. If opts is nil, the default options are used.
func NewHandler(w io.Writer, opts *Options) slog.Handler {
	h := &handler{
		w:          w,
		level:      defaultLevel,
		timeFormat: defaultTimeFormat,
	}
	if opts == nil {
		return h
	}

	h.addSource = opts.AddSource
	if opts.Level != nil {
		h.level = opts.Level
	}
	h.replaceAttr = opts.ReplaceAttr
	if opts.TimeFormat != "" {
		h.timeFormat = opts.TimeFormat
	}
	h.noColor = opts.NoColor
	return h
}

// handler implements a [slog.Handler].
type handler struct {
	attrsPrefix string
	groupPrefix string
	groups      []string

	mu sync.Mutex
	w  io.Writer

	addSource   bool
	level       slog.Leveler
	replaceAttr func([]string, slog.Attr) slog.Attr
	timeFormat  string
	noColor     bool
}

// clone returns a shallow copy of the handler
func (h *handler) clone() *handler {
	return &handler{
		attrsPrefix: h.attrsPrefix,
		groupPrefix: h.groupPrefix,
		groups:      h.groups,
		w:           h.w,
		addSource:   h.addSource,
		level:       h.level,
		replaceAttr: h.replaceAttr,
		timeFormat:  h.timeFormat,
		noColor:     h.noColor,
	}
}

// Enabled returns true if the level is enabled
func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle writes a log record to the handler's writer
func (h *handler) Handle(_ context.Context, r slog.Record) error {
	// get a buffer from the sync pool
	buf := newBuffer()
	defer buf.Free()

	rep := h.replaceAttr

	// write time
	if !r.Time.IsZero() {
		val := r.Time.Round(0) // strip monotonic to match Attr behavior
		if rep == nil {
			h.appendTime(buf, r.Time)
			buf.WriteChar(' ')
		} else if a := rep(nil /* groups */, slog.Time(slog.TimeKey, val)); a.Key != "" {
			if a.Value.Kind() == slog.KindTime {
				h.appendTime(buf, a.Value.Time())
			} else {
				h.appendValue(buf, a.Value, false)
			}
			buf.WriteChar(' ')
		}
	}

	// write level
	if rep == nil {
		h.appendLevel(buf, r.Level)
		buf.WriteChar(' ')
	} else if a := rep(nil /* groups */, slog.Any(slog.LevelKey, r.Level)); a.Key != "" {
		h.appendValue(buf, a.Value, false)
		buf.WriteChar(' ')
	}

	// write source
	if h.addSource {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		if f.File != "" {
			src := &slog.Source{
				Function: f.Function,
				File:     f.File,
				Line:     f.Line,
			}

			if rep == nil {
				h.appendSource(buf, src)
				buf.WriteChar(' ')
			} else if a := rep(nil /* groups */, slog.Any(slog.SourceKey, src)); a.Key != "" {
				h.appendValue(buf, a.Value, false)
				buf.WriteChar(' ')
			}
		}
	}

	// write message
	if rep == nil {
		buf.WriteString(r.Message)
		buf.WriteChar(' ')
	} else if a := rep(nil /* groups */, slog.String(slog.MessageKey, r.Message)); a.Key != "" {
		h.appendValue(buf, a.Value, false)
		buf.WriteChar(' ')
	}

	// write handler attributes
	if len(h.attrsPrefix) > 0 {
		buf.WriteString(h.attrsPrefix)
	}

	// write attributes
	r.Attrs(func(attr slog.Attr) bool {
		h.appendAttr(buf, attr, h.groupPrefix, h.groups)
		return true
	})

	if len(*buf) == 0 {
		return nil
	}
	(*buf)[len(*buf)-1] = '\n' // replace last space with newline

	h.mu.Lock()
	defer h.mu.Unlock()

	_, err := h.w.Write(*buf)
	return err
}

// WithAttrs returns a new handler with the given attributes
func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	h2 := h.clone()

	buf := newBuffer()
	defer buf.Free()

	// write attributes to buffer
	for _, attr := range attrs {
		h.appendAttr(buf, attr, h.groupPrefix, h.groups)
	}
	h2.attrsPrefix = h.attrsPrefix + string(*buf)
	return h2
}

// WithGroup returns a new handler with the given group name
func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.groupPrefix += name + "."
	h2.groups = append(h2.groups, name)
	return h2
}

// appendTime appends a time to the buffer
func (h *handler) appendTime(buf *buffer, t time.Time) {
	buf.WriteStringIf(!h.noColor, ansiFaint)
	*buf = t.AppendFormat(*buf, h.timeFormat)
	buf.WriteStringIf(!h.noColor, ansiReset)
}

// appendLevel appends a level to the buffer
func (h *handler) appendLevel(buf *buffer, level slog.Level) {
	switch {
	case level < slog.LevelInfo:
		buf.WriteStringIf(!h.noColor, ansiBrightMagentaFaint)
		buf.WriteString("DBG")
		appendLevelDelta(buf, level-slog.LevelDebug)
		buf.WriteStringIf(!h.noColor, ansiReset)
	case level < slog.LevelWarn:
		buf.WriteStringIf(!h.noColor, ansiBrightGreen)
		buf.WriteString("INF")
		appendLevelDelta(buf, level-slog.LevelInfo)
		buf.WriteStringIf(!h.noColor, ansiReset)
	case level < slog.LevelError:
		buf.WriteStringIf(!h.noColor, ansiBrightYellow)
		buf.WriteString("WRN")
		appendLevelDelta(buf, level-slog.LevelWarn)
		buf.WriteStringIf(!h.noColor, ansiReset)
	default:
		buf.WriteStringIf(!h.noColor, ansiBrightRed)
		buf.WriteString("ERR")
		appendLevelDelta(buf, level-slog.LevelError)
		buf.WriteStringIf(!h.noColor, ansiReset)
	}
}

// appendLevelDelta appends a level delta to the buffer
func appendLevelDelta(buf *buffer, delta slog.Level) {
	if delta == 0 {
		return
	} else if delta > 0 {
		buf.WriteChar('+')
	}
	*buf = strconv.AppendInt(*buf, int64(delta), 10)
}

// appendSource appends source details to the buffer
func (h *handler) appendSource(buf *buffer, src *slog.Source) {
	dir, file := filepath.Split(src.File)

	buf.WriteStringIf(!h.noColor, ansiFaint)
	buf.WriteString(filepath.Join(filepath.Base(dir), file))
	buf.WriteChar(':')
	buf.WriteString(strconv.Itoa(src.Line))
	buf.WriteStringIf(!h.noColor, ansiReset)
}

// appendAttr appends an attribute to the buffer
func (h *handler) appendAttr(buf *buffer, attr slog.Attr, groupsPrefix string, groups []string) {
	attr.Value = attr.Value.Resolve()
	if rep := h.replaceAttr; rep != nil && attr.Value.Kind() != slog.KindGroup {
		attr = rep(groups, attr)
		attr.Value = attr.Value.Resolve()
	}

	if attr.Equal(slog.Attr{}) {
		return
	}

	if attr.Value.Kind() == slog.KindGroup {
		if attr.Key != "" {
			groupsPrefix += attr.Key + "."
			groups = append(groups, attr.Key)
		}
		for _, groupAttr := range attr.Value.Group() {
			h.appendAttr(buf, groupAttr, groupsPrefix, groups)
		}
	} else if err, ok := attr.Value.Any().(error); ok {
		h.appendError(buf, err, attr.Key, groupsPrefix)
		buf.WriteChar(' ')
	} else {
		h.appendKey(buf, attr.Key, groupsPrefix)
		h.appendValue(buf, attr.Value, true)
		buf.WriteChar(' ')
	}
}

// appendKey appends a key to the buffer
func (h *handler) appendKey(buf *buffer, key, groups string) {
	buf.WriteStringIf(!h.noColor, ansiFaint)
	appendString(buf, groups+key, true)
	buf.WriteChar('=')
	buf.WriteStringIf(!h.noColor, ansiReset)
}

// appendValue appends a value to the buffer
func (h *handler) appendValue(buf *buffer, v slog.Value, quote bool) {
	switch v.Kind() {
	case slog.KindString:
		appendString(buf, v.String(), quote)
	case slog.KindInt64:
		*buf = strconv.AppendInt(*buf, v.Int64(), 10)
	case slog.KindUint64:
		*buf = strconv.AppendUint(*buf, v.Uint64(), 10)
	case slog.KindFloat64:
		*buf = strconv.AppendFloat(*buf, v.Float64(), 'g', -1, 64)
	case slog.KindBool:
		*buf = strconv.AppendBool(*buf, v.Bool())
	case slog.KindDuration:
		appendString(buf, v.Duration().String(), quote)
	case slog.KindTime:
		appendString(buf, v.Time().String(), quote)
	case slog.KindAny:
		switch cv := v.Any().(type) {
		case slog.Level:
			h.appendLevel(buf, cv)
		case encoding.TextMarshaler:
			data, err := cv.MarshalText()
			if err != nil {
				break
			}
			appendString(buf, string(data), quote)
		case *slog.Source:
			h.appendSource(buf, cv)
		default:
			appendString(buf, fmt.Sprintf("%+v", v.Any()), quote)
		}
	}
}

// appendError appends an error to the buffer
func (h *handler) appendError(buf *buffer, err error, attrKey, groupsPrefix string) {
	buf.WriteStringIf(!h.noColor, ansiBrightRedFaint)
	appendString(buf, groupsPrefix+attrKey, true)
	buf.WriteChar('=')
	buf.WriteStringIf(!h.noColor, ansiResetFaint)
	appendString(buf, err.Error(), true)
	buf.WriteStringIf(!h.noColor, ansiReset)
}

// appendString appends a string to the buffer
func appendString(buf *buffer, s string, quote bool) {
	if quote && needsQuoting(s) {
		*buf = strconv.AppendQuote(*buf, s)
	} else {
		buf.WriteString(s)
	}
}

// needsQuoting returns true if the string needs quoting
func needsQuoting(s string) bool {
	if len(s) == 0 {
		return true
	}
	for _, r := range s {
		if unicode.IsSpace(r) || r == '"' || r == '=' || !unicode.IsPrint(r) {
			return true
		}
	}
	return false
}
