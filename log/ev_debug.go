package log

import (
	"github.com/rs/zerolog"
)

func Debug(logger *zerolog.Logger) *zerolog.Event {
	return logger.Debug()
}
