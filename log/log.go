package log

import "github.com/rs/zerolog"

type Loggable interface {
	MarshalEvent(ev *zerolog.Event) error
}

// MustLoggableTo does NOT panic.
func MustLoggableTo(to zerolog.Logger, level zerolog.Level, msg string, loggable Loggable) {
	if err := LoggableTo(to, level, msg, loggable); err != nil {
		to.Error().Err(err).Msg("Failed to log")
	}
}

func LoggableTo(to zerolog.Logger, level zerolog.Level, msg string, loggable Loggable) error {
	return EventTo(to.WithLevel(level), msg, loggable)
}

func EventTo(ev *zerolog.Event, msg string, loggable Loggable) error {
	if err := loggable.MarshalEvent(ev); err != nil {
		return err
	}

	ev.Msg(msg)
	return nil
}
