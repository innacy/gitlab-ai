package utils

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger is the global logger instance.
var Logger zerolog.Logger

// InitLogger sets up the global zerolog logger.
func InitLogger(verbose bool) {
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
	}

	if verbose {
		Logger = zerolog.New(consoleWriter).
			With().
			Timestamp().
			Caller().
			Logger().
			Level(zerolog.DebugLevel)
	} else {
		Logger = zerolog.New(consoleWriter).
			With().
			Timestamp().
			Logger().
			Level(zerolog.InfoLevel)
	}
}

// Debug logs a debug message.
func Debug(msg string) {
	Logger.Debug().Msg(msg)
}

// Debugf logs a formatted debug message.
func Debugf(format string, args ...interface{}) {
	Logger.Debug().Msgf(format, args...)
}
