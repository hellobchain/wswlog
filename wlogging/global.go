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
	"time"

	rotatelogs "github.com/hellobchain/wswlog/file-rotatelogs"
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

func getHook(filename string, maxAge, rotationTime int, rotationSize int64) (io.Writer, error) {

	hook, err := rotatelogs.New(
		filename+".%Y%m%d%H",
		rotatelogs.WithRotationTime(time.Hour*time.Duration(rotationTime)),
		rotatelogs.WithLinkName(filename),
		rotatelogs.WithMaxAge(time.Hour*24*time.Duration(maxAge)),
	)

	if err != nil {
		return nil, err
	}

	return hook, nil
}

// 日志切割默认配置
const (
	DEFAULT_MAX_AGE       = 7   // 日志最长保存时间，单位：天
	DEFAULT_ROTATION_TIME = 6   // 日志滚动间隔，单位：小时
	DEFAULT_ROTATION_SIZE = 100 // 默认的日志滚动大小，单位：MB
)

type LogConfig struct {
	LogPath      string // logPath: log file save path
	MaxAge       int    // maxAge: the maximum number of days to retain old log files
	RotationTime int    // RotationTime: rotation time
	RotationSize int64  // RotationSize: rotation size Mb
	Console      bool   // console: whether to print to the console
}

func MustGetFileLoggerWithoutName(logConfig *LogConfig) *WswLogger {
	pc, _, _, _ := runtime.Caller(1)
	funcObj := runtime.FuncForPC(pc)
	r := regexp.MustCompile(`^.*/(.*)*\..*$`)
	name := r.ReplaceAllString(funcObj.Name(), "$1")
	if logConfig == nil {
		logConfig = &LogConfig{
			LogPath:      "log/wsw.log",
			MaxAge:       DEFAULT_MAX_AGE,
			RotationTime: DEFAULT_ROTATION_TIME,
			RotationSize: DEFAULT_ROTATION_SIZE,
		}
	}
	hook, err := getHook(logConfig.LogPath, logConfig.MaxAge, logConfig.RotationTime, logConfig.RotationSize)
	if err != nil {
		panic(err)
	}
	Global.SetWriter(hook)
	Global.SetConsole(logConfig.Console)
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

func SetConsole(console bool) bool {
	return Global.SetConsole(console)
}

func SetDefaultWriter(w io.Writer) {
	defaultWriter = w
}
