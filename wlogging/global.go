/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package wlogging

import (
	"io"
	"regexp"
	"runtime"
	"strings"

	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/grpclog"
)

const (
	defaultFormat = "%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}"
	defaultLevel  = zapcore.InfoLevel
)

var Global *Logging
var logger *WswLogger

func init() {
	logging, err := New(Config{})
	if err != nil {
		panic(err)
	}

	Global = logging
	logger = Global.Logger("flogging")
	grpcLogger := Global.ZapLogger("grpc")
	grpclog.SetLogger(NewGRPCLogger(grpcLogger))
}

// Init initializes logging with the provided config.
func Init(config Config) {
	err := Global.Apply(config)
	if err != nil {
		panic(err)
	}
}

// Reset sets logging to the defaults defined in this package.
//
// Used in tests and in the package init
func Reset() {
	_ = Global.Apply(Config{})
}

// LoggerLevel gets the current logging level for the logger with the
// provided name.
func LoggerLevel(loggerName string) string {
	return strings.ToUpper(Global.Level(loggerName).String())
}

// MustGetLogger creates a logger with the specified name. If an invalid name
// is provided, the operation will panic.
func MustGetLogger(loggerName string) *WswLogger {
	return Global.Logger(loggerName)
}

// 自动获取包名
func MustGetLoggerWithoutName() *WswLogger {
	pc, _, _, _ := runtime.Caller(1)
	funcObj := runtime.FuncForPC(pc)
	r := regexp.MustCompile(`^.*/(.*)*\..*$`)
	name := r.ReplaceAllString(funcObj.Name(), "$1")
	return Global.Logger(name)
}

// ActivateSpec is used to activate a logging specification.
func ActivateSpec(spec string) {
	err := Global.ActivateSpec(spec)
	if err != nil {
		panic(err)
	}
}

func SetGlobalLogLevel(level string) {
	err := Global.ActivateSpec(level)
	if err != nil {
		logger.Warning("set log level unknown ,set default (info)")
		_ = Global.ActivateSpec("info")
	}

}

// SetWriter calls SetWriter returning the previous value
// of the writer.
func SetWriter(w io.Writer) io.Writer {
	return Global.SetWriter(w)
}

// SetObserver calls SetObserver returning the previous value
// of the observer.
func SetObserver(observer Observer) Observer {
	return Global.SetObserver(observer)
}
