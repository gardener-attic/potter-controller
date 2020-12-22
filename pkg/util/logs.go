package util

import (
	"context"

	"github.com/go-logr/logr"
)

func GetLoggerFromContext(ctx context.Context) logr.Logger {
	return ctx.Value(LoggerKey{}).(logr.Logger)
}

func NewContextAndLogger(logger logr.Logger, keysAndValues ...interface{}) (context.Context, logr.Logger) {
	logger = logger.WithValues(keysAndValues...)
	return context.WithValue(context.Background(), LoggerKey{}, logger), logger
}

func EnrichContextAndLogger(ctx context.Context, keysAndValues ...interface{}) (context.Context, logr.Logger) {
	logger := GetLoggerFromContext(ctx).WithValues(keysAndValues...)
	return context.WithValue(ctx, LoggerKey{}, logger), logger
}
