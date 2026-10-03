package logger

import (
	"errors"
	"os"
	"regexp"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Init(logLevel string) {
	if os.Getenv("ENV") == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	// Set log level
	switch logLevel {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

var sensitiveValuePattern = regexp.MustCompile(`(?i)(access[_-]?token|refresh[_-]?token|authorization|bearer|client[_-]?secret|oauth[_-]?token)(\s*[:=]\s*|\s+)([^\s,;]+)`)

func WithJobContext(jobID, postID, userID, platform string, attempt int) zerolog.Logger {
	ctx := log.With()
	if jobID != "" {
		ctx = ctx.Str("job_id", jobID)
	}
	if postID != "" {
		ctx = ctx.Str("post_id", postID)
	}
	if userID != "" {
		ctx = ctx.Str("user_id", userID)
	}
	if platform != "" {
		ctx = ctx.Str("platform", platform)
	}
	if attempt > 0 {
		ctx = ctx.Int("attempt", attempt)
	}
	return ctx.Logger()
}

func RedactSecrets(value string) string {
	if value == "" {
		return ""
	}
	return strings.TrimSpace(sensitiveValuePattern.ReplaceAllString(value, `${1}${2}[REDACTED]`))
}

func RedactError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(RedactSecrets(err.Error()))
}

// Aliases

func Debug(msg string) {
	log.Debug().Msg(msg)
}

func Info(msg string) {
	log.Info().Msg(msg)
}

func Error(err error, msg string) {
	log.Error().Err(RedactError(err)).Msg(msg)
}

func Fatal(err error, msg string) {
	log.Fatal().Err(RedactError(err)).Msg(msg)
}