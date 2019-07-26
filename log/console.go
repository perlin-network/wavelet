// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

const (
	colorBold = iota + 1
	colorFaint
)

const (
	colorRed = iota + 31
	colorGreen
	colorYellow
)

var (
	consoleBufPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 100))
		},
	}

	consoleDefaultTimeFormat = time.Kitchen

	consoleDefaultPartsOrder = func() []string {
		return []string{
			zerolog.TimestampFieldName,
			zerolog.LevelFieldName,
			zerolog.CallerFieldName,
			zerolog.MessageFieldName,
		}
	}

	consoleNoColor    = false
	consoleTimeFormat = consoleDefaultTimeFormat
)

// Formatter transforms the input into a formatted string.
type Formatter func(interface{}) string

// ConsoleWriter parses the JSON input and writes it in an
// (optionally) colorized, human-friendly format to Out.
type ConsoleWriter struct {
	// Out is the output destination.
	Out io.Writer

	// NoColor disables the colorized output.
	NoColor bool

	// TimeFormat specifies the format for timestamp in output.
	TimeFormat string

	// PartsOrder defines the order of parts in output.
	PartsOrder []string

	FormatTimestamp     Formatter
	FormatLevel         Formatter
	FormatCaller        Formatter
	FormatMessage       Formatter
	FormatFieldName     Formatter
	FormatFieldValue    Formatter
	FormatErrFieldName  Formatter
	FormatErrFieldValue Formatter

	FilteredModules map[string]struct{}
}

func FilterFor(modules ...string) func(w *ConsoleWriter) {
	return func(w *ConsoleWriter) {
		for _, module := range modules {
			w.FilteredModules[module] = struct{}{}
		}
	}
}

// NewConsoleWriter creates and initializes a new ConsoleWriter.
func NewConsoleWriter(writer io.Writer, options ...func(w *ConsoleWriter)) ConsoleWriter {
	if writer == nil {
		writer = os.Stdout
	}

	w := ConsoleWriter{
		Out:             writer,
		TimeFormat:      consoleDefaultTimeFormat,
		PartsOrder:      consoleDefaultPartsOrder(),
		FilteredModules: make(map[string]struct{}),
	}

	for _, opt := range options {
		opt(&w)
	}

	return w
}

// Write transforms the JSON input with formatters and appends to w.Out.
func (w ConsoleWriter) Write(p []byte) (n int, err error) {
	if w.PartsOrder == nil {
		w.PartsOrder = consoleDefaultPartsOrder()
	}
	if w.TimeFormat == "" && consoleTimeFormat != consoleDefaultTimeFormat {
		consoleTimeFormat = consoleDefaultTimeFormat
	}
	if w.TimeFormat != "" && consoleTimeFormat != w.TimeFormat {
		consoleTimeFormat = w.TimeFormat
	}
	if !w.NoColor && consoleNoColor {
		consoleNoColor = false
	}
	if w.NoColor && consoleNoColor != w.NoColor {
		consoleNoColor = w.NoColor
	}

	var buf = consoleBufPool.Get().(*bytes.Buffer)
	defer consoleBufPool.Put(buf)

	var event map[string]interface{}

	decoder := json.NewDecoder(bytes.NewReader(p))
	decoder.UseNumber()

	err = decoder.Decode(&event)
	if err != nil {
		return n, fmt.Errorf("cannot decode event: %s", err)
	}

	if module := event[KeyModule]; module != nil {
		if _, filtered := w.FilteredModules[module.(string)]; !filtered {
			return len(p), nil
		}
	}

	delete(event, KeyModule)

	for _, p := range w.PartsOrder {
		w.writePart(buf, event, p)
	}

	w.writeFields(event, buf)

	_ = buf.WriteByte('\n')
	_, _ = buf.WriteTo(w.Out)

	return len(p), nil
}

// writeFields appends formatted key-value pairs to buf.
func (w ConsoleWriter) writeFields(evt map[string]interface{}, buf *bytes.Buffer) {
	var fields = make([]string, 0, len(evt))
	for field := range evt {
		switch field {
		case zerolog.LevelFieldName, zerolog.TimestampFieldName, zerolog.MessageFieldName, zerolog.CallerFieldName:
			continue
		}
		fields = append(fields, field)
	}
	sort.Strings(fields)

	if len(fields) > 0 {
		buf.Write([]byte("\n\t"))
	}

	// Move the "error" field to the front
	ei := sort.Search(len(fields), func(i int) bool { return fields[i] >= zerolog.ErrorFieldName })
	if ei < len(fields) && fields[ei] == zerolog.ErrorFieldName {
		fields[ei] = ""
		fields = append([]string{zerolog.ErrorFieldName}, fields...)
		var xfields = make([]string, 0, len(fields))
		for _, field := range fields {
			if field == "" { // Skip empty fields
				continue
			}
			xfields = append(xfields, field)
		}
		fields = xfields
	}

	for i, field := range fields {
		var fn Formatter
		var fv Formatter

		if field == zerolog.ErrorFieldName {
			if w.FormatErrFieldName == nil {
				fn = consoleDefaultFormatErrFieldName
			} else {
				fn = w.FormatErrFieldName
			}

			if w.FormatErrFieldValue == nil {
				fv = consoleDefaultFormatErrFieldValue
			} else {
				fv = w.FormatErrFieldValue
			}
		} else {
			if w.FormatFieldName == nil {
				fn = consoleDefaultFormatFieldName
			} else {
				fn = w.FormatFieldName
			}

			if w.FormatFieldValue == nil {
				fv = consoleDefaultFormatFieldValue
			} else {
				fv = w.FormatFieldValue
			}
		}

		// Insert the field key
		buf.WriteString(fn(field))

		switch fValue := evt[field].(type) {
		case string:
			if needsQuote(fValue) {
				buf.WriteString(fv(strconv.Quote(fValue)))
			} else {
				buf.WriteString(fv(fValue))
			}
		case json.Number:
			buf.WriteString(fv(fValue))
		default:
			b, err := json.Marshal(fValue)
			if err != nil {
				_, _ = fmt.Fprintf(buf, colorize("[error: %v]", colorRed, w.NoColor), err)
			} else {
				_, _ = fmt.Fprint(buf, fv(b))
			}
		}

		if i < len(fields)-1 { // Skip space for last field
			buf.WriteByte(' ')
		}
	}
}

// writePart appends a formatted part to buf.
func (w ConsoleWriter) writePart(buf *bytes.Buffer, evt map[string]interface{}, p string) {
	var f Formatter

	switch p {
	case zerolog.LevelFieldName:
		if w.FormatLevel == nil {
			f = consoleDefaultFormatLevel
		} else {
			f = w.FormatLevel
		}
	case zerolog.TimestampFieldName:
		if w.FormatTimestamp == nil {
			f = consoleDefaultFormatTimestamp
		} else {
			f = w.FormatTimestamp
		}
	case zerolog.MessageFieldName:
		if w.FormatMessage == nil {
			f = consoleDefaultFormatMessage
		} else {
			f = w.FormatMessage
		}
	case zerolog.CallerFieldName:
		if w.FormatCaller == nil {
			f = consoleDefaultFormatCaller
		} else {
			f = w.FormatCaller
		}
	default:
		if w.FormatFieldValue == nil {
			f = consoleDefaultFormatFieldValue
		} else {
			f = w.FormatFieldValue
		}
	}

	var s = f(evt[p])

	if len(s) > 0 {
		buf.WriteString(s)
		if p != w.PartsOrder[len(w.PartsOrder)-1] { // Skip space for last part
			buf.WriteByte(' ')
		}
	}
}

// needsQuote returns true when the string s should be quoted in output.
func needsQuote(s string) bool {
	for i := range s {
		if s[i] < 0x20 || s[i] > 0x7e || s[i] == ' ' || s[i] == '\\' || s[i] == '"' {
			return true
		}
	}
	return false
}

// colorize returns the string s wrapped in ANSI code c, unless disabled is true.
func colorize(s interface{}, c int, disabled bool) string {
	if disabled {
		return fmt.Sprintf("%s", s)
	}
	return fmt.Sprintf("\x1b[%dm%v\x1b[0m", c, s)
}

// ----- DEFAULT FORMATTERS ---------------------------------------------------

var (
	consoleDefaultFormatTimestamp = func(i interface{}) string {
		t := "<nil>"
		switch tt := i.(type) {
		case string:
			ts, err := time.Parse(time.RFC3339, tt)
			if err != nil {
				t = tt
			} else {
				t = ts.Format(consoleTimeFormat)
			}
		case json.Number:
			t = tt.String()
		}
		return colorize(t, colorFaint, consoleNoColor)
	}

	consoleDefaultFormatLevel = func(i interface{}) string {
		var l string
		if ll, ok := i.(string); ok {
			switch ll {
			case "debug":
				l = colorize("DBG", colorYellow, consoleNoColor)
			case "info":
				l = colorize("INF", colorGreen, consoleNoColor)
			case "warn":
				l = colorize("WRN", colorRed, consoleNoColor)
			case "error":
				l = colorize(colorize("ERR", colorRed, consoleNoColor), colorBold, consoleNoColor)
			case "fatal":
				l = colorize(colorize("FTL", colorRed, consoleNoColor), colorBold, consoleNoColor)
			case "panic":
				l = colorize(colorize("PNC", colorRed, consoleNoColor), colorBold, consoleNoColor)
			default:
				l = colorize("???", colorBold, consoleNoColor)
			}
		} else {
			l = ""
		}
		return l
	}

	consoleDefaultFormatCaller = func(i interface{}) string {
		var c string
		if cc, ok := i.(string); ok {
			c = cc
		}
		if len(c) > 0 {
			cwd, err := os.Getwd()
			if err == nil {
				c = strings.TrimPrefix(c, cwd)
				c = strings.TrimPrefix(c, "/")
			}
			c = colorize(c, colorBold, consoleNoColor) + colorize(" >", colorFaint, consoleNoColor)
		}
		return c
	}

	consoleDefaultFormatMessage = func(i interface{}) string {
		return fmt.Sprintf("%s", i)
	}

	consoleDefaultFormatFieldName = func(i interface{}) string {
		return colorize(fmt.Sprintf("%s: ", i), colorFaint, consoleNoColor)
	}

	consoleDefaultFormatFieldValue = func(i interface{}) string {
		return fmt.Sprintf("%s", i)
	}

	consoleDefaultFormatErrFieldName = func(i interface{}) string {
		return colorize(fmt.Sprintf("%s: ", i), colorRed, consoleNoColor)
	}

	consoleDefaultFormatErrFieldValue = func(i interface{}) string {
		return colorize(fmt.Sprintf("%s", i), colorRed, consoleNoColor)
	}
)
