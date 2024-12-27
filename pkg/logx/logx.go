package logx

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger = initLogger()

func initLogger() *zap.SugaredLogger {
	var logger *zap.Logger
	if false {
		devConf := zap.NewDevelopmentConfig()
		devConf.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		logger = zap.Must(devConf.Build())
	} else {
		prodConf := zap.NewProductionConfig()
		prodConf.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		logger = zap.Must(prodConf.Build())
	}
	return logger.Sugar()
}

type Logx struct{}

func ContextWithLogger(ctx context.Context, l *zap.SugaredLogger) context.Context {
	return context.WithValue(ctx, Logx{}, l)
}

func LoggerFromContext(ctx context.Context) *zap.SugaredLogger {
	if l, ok := ctx.Value(Logx{}).(*zap.SugaredLogger); ok {
		return l
	}
	return Logger
}
