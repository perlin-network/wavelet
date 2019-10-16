package logger

import (
	"fmt"
	"strings"
	"time"

	"github.com/diamondburned/tview/v2"
)

// Level provides a logging level interface. An example for using Error:
//
//    l.Level(logger.WithError(err).Wrap("An error has occured").
//        F("id", "%d", id))
//
/*
type Level interface {
	Full() string
	Short() (string, string)
	F(key, format string, values ...interface{}) Level
}
*/

type Level string

const (
	Error   Level = "ERROR"
	Warning Level = "WARNING"
	Info    Level = "INFO"
	Success Level = "SUCCESS"
)

var Colors = map[Level]string{
	Error:   "red",
	Warning: "yellow",
	Info:    "teal",
	Success: "lime",
}

type Event struct {
	Fields [][2]string
	Level  Level
	Msg    string
	Time   time.Time

	// Only used when Level is Error, msg will be used to wrap
	Err error
}

func (ev *Event) Full() string {
	var s strings.Builder
	s.WriteString(ev.Time.Format(time.Stamp) + " - [" + Colors[ev.Level] + "]")

	switch ev.Level {
	case Error:
		s.WriteString(string(ev.Level) + ": ")
		if ev.Msg != "" {
			s.WriteString(ev.Msg + ": ")
		}
		s.WriteString(ev.Err.Error() + "\n")
	default:
		s.WriteString(string(ev.Level) + ": " + ev.Msg + "\n")
	}

	for _, f := range ev.Fields {
		s.WriteString("[-]" + "\t" + f[0] + " = " + f[1] + "\n")
	}

	s.WriteString("[-]")
	return s.String()
}

func (ev *Event) Short() (title, desc string) {
	// Set the title, something like "4:00AM - "
	title = ev.Time.Format(time.Kitchen) + " - [" + Colors[ev.Level] + "]"
	title += string(ev.Level) + ": "

	switch ev.Level {
	case Error:
		if ev.Msg != "" {
			title += ev.Msg + ": "
		}

		title += ev.Err.Error()
	default:
		title += ev.Msg
	}

	if len(ev.Fields) > 0 {
		for _, f := range ev.Fields {
			var val = f[1]
			if len(val) > 8 {
				val = val[:8] + "…"
			}

			desc += "[-]" + f[0] + "=" + val + ", "
		}

		// len - 2 removes the trailing comma
		desc = "        " + desc[:len(desc)-2] + "[-]"
	}

	return
}

func F(key, format string, v ...interface{}) [2]string {
	return [2]string{
		tview.Escape(key),
		tview.Escape(fmt.Sprintf(format, v...)),
	}
}

func Ln(key string, v ...interface{}) [2]string {
	return [2]string{
		tview.Escape(key),
		tview.Escape(fmt.Sprint(v...)),
	}
}

func (l *Logger) Error(err error, fields ...[2]string) {
	l.SendEv(&Event{
		Level:  Error,
		Fields: fields,
		Err:    err,
		Time:   time.Now(),
	})
}

func (l *Logger) Wrap(err error, msg string, fields ...[2]string) {
	l.SendEv(&Event{
		Level:  Error,
		Fields: fields,
		Msg:    msg,
		Err:    err,
		Time:   time.Now(),
	})
}

func (l *Logger) Level(level Level, msg string, fields ...[2]string) {
	l.SendEv(&Event{
		Level:  level,
		Msg:    msg,
		Fields: fields,
		Time:   time.Now(),
	})
}

func (l *Logger) SendEv(ev *Event) {
	l.evMutex.Lock()
	defer l.evMutex.Unlock()

	// Add the event to the state list
	l.evs = append(l.evs, ev)

	// Add the event to the UI list as well
	m, s := ev.Short()
	l.list.AddItem(m, s, 0, nil)

	// Get the information to estimate scroll
	i := l.list.GetCurrentItem()
	c := l.list.GetItemCount()

	if (i > c-3) && l.activeEv == nil || !l.focus {
		// The Logger is not focused, or the cursor is around the list and we're
		// still on the list, just scroll to bottom.
		l.list.SetCurrentItem(c - 1)
	}

	// Draw the application
	tview.Draw()
}
