# `tinter`: 🌈 **slog.Handler** that writes tinted logs

[![Go Reference](https://pkg.go.dev/badge/github.com/pwntr/tinter.svg)](https://pkg.go.dev/github.com/pwntr/tinter#section-documentation)
[![Go Report Card](https://goreportcard.com/badge/github.com/pwntr/tinter)](https://goreportcard.com/report/github.com/pwntr/tinter)

Package `tinter` implements a zero-dependency [`slog.Handler`](https://pkg.go.dev/log/slog#Handler) that writes tinted (colorized) logs. Its output format is inspired by the `zerolog.ConsoleWriter` and [`slog.TextHandler`](https://pkg.go.dev/log/slog#TextHandler).

The output format can be customized using [`Options`](https://pkg.go.dev/github.com/pwntr/tinter#Options), which is a drop-in replacement for [`slog.HandlerOptions`](https://pkg.go.dev/log/slog#HandlerOptions).

To get started, run:
```
go get github.com/pwntr/tinter
```

## Output preview:

![image](https://github.com/user-attachments/assets/b6afee00-7d9b-43fa-aec8-c0751900f8f0)

## About

`tinter` is a fork of `lmittmann/tint`, which introduces the following changes:

* Uses go-native `error` values only (anything that implements the `error` interface), which are passed in as k/v pairs or `slog.Attr` attributes to colorize any errors that appear in the log, not only errors that had to be previously wrapped explicitly (ref. to the original [`tint.Err` type wrapper](https://github.com/lmittmann/tint/blob/368de753ea2a714981dac3bed7390460b9ae4a93/handler.go#L427)).

    The key for the `error` you pass can be anything you like, as long as the value implements the `error` interface. 

* Uses an opinionated faint magenta as the foreground color of the debug level text, to increase readability when glancing over logs, by visually separating it more clearly from the log message itself.

* Eventually handle all errors in the current code (meaning, don't ignore returned errors), so that the linters (and users) are happy.

The primary goal of this fork is to have a tinted logger avaiable that doesn't require exposure of the `tinter` API to deeper levels of your own business logic or logging API surface - it's a simple drop-in handler that you initialize once with your logger instance and are done. The rest of your app then logs as normal with the default `slog` API surface.

### Note to users migrating from `lmittmann/tint`

Due to the removal of the `tint.Err` type wrapper, this fork of `tint` introduced a breaking change. In case your logging calls actually used that wrapper, please just use the go-native `error` type for the value (or anything that implements the `error` interface), in combination with any key you like, or using attributes like `slog.Any(...)` of type `slog.Attr`. See [Usage](#usage) for some example logging calls.

The versioning for `tinter` starts at `v1.0.0` with this fork.

## Usage

```go
import (
    ...
    "log/slog"
    "github.com/pwntr/tinter"
    ...
)

...

// log target
w := os.Stderr

// logger options
opts := &tinter.Options{
        Level:      slog.LevelDebug,
        TimeFormat: time.Kitchen,
    }

// create a new logger
logger := slog.New(tinter.NewHandler(w, opts))

// set global logger to our new logger (or pass the logger where you need it)
slog.SetDefault(logger)

// example logging calls
slog.Info("Starting server", "addr", ":8080", "env", "production")
slog.Debug("Connected to DB", "db", "myapp", "host", "localhost:5432")
slog.Warn("Slow request", "method", "GET", "path", "/users", "duration", 497*time.Millisecond)
slog.Error("DB connection lost", "err", errors.New("connection reset"), "db", "myapp")
slog.Error("Flux capacitor gone", slog.Any("fail", errors.New("arbitrary error passed")), "engine", 42)

...
```

### Customize Attributes

`ReplaceAttr` can be used to alter or drop attributes. If set, it is called on
each non-group attribute before it is logged. See [`slog.HandlerOptions`](https://pkg.go.dev/log/slog#HandlerOptions)
for details.

```go
// create a new logger that doesn't write the time
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
```

### Automatically Enable Colors

Colors are enabled by default and can be disabled using the `Options.NoColor`
attribute. To automatically enable colors based on the terminal capabilities,
use e.g. the [`go-isatty`](https://github.com/mattn/go-isatty) package.

```go
w := os.Stderr
logger := slog.New(
    tinter.NewHandler(w, &tinter.Options{
        NoColor: !isatty.IsTerminal(w.Fd()),
    }),
)
```

### Windows Support

Color support on Windows can be added by using e.g. the
[`go-colorable`](https://github.com/mattn/go-colorable) package.

```go
w := os.Stderr
logger := slog.New(
    tinter.NewHandler(colorable.NewColorable(w), nil),
)
```
