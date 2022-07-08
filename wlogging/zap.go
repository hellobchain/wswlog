/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package wlogging

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapgrpc"
)

// NewZapLogger creates a zap logger around a new zap.Core. The core will use
// the provided encoder and sinks and a level enabler that is associated with
// the provided logger name. The logger that is returned will be named the same
// as the logger.
func NewZapLogger(core zapcore.Core, options ...zap.Option) *zap.Logger {
	return zap.New(
		core,
		append([]zap.Option{
			zap.AddCaller(),
			zap.AddStacktrace(zapcore.ErrorLevel),
		}, options...)...,
	)
}

// NewGRPCLogger creates a grpc.Logger that delegates to a zap.Logger.
func NewGRPCLogger(l *zap.Logger) *zapgrpc.Logger {
	l = l.WithOptions(
		zap.AddCaller(),
		zap.AddCallerSkip(3),
	)
	return zapgrpc.NewLogger(l, zapgrpc.WithDebug())
}

// NewWswLogger creates a logger that delegates to the zap.SugaredLogger.
func NewWswLogger(l *zap.Logger, options ...zap.Option) *WswLogger {
	return &WswLogger{
		s: l.WithOptions(append(options, zap.AddCallerSkip(1))...).Sugar(),
	}
}

// A WswLogger is an adapter around a zap.SugaredLogger that provides
// structured logging capabilities while preserving much of the legacy logging
// behavior.
//
// The most significant difference between the WswLogger and the
// zap.SugaredLogger is that methods without a formatting suffix (f or w) build
// the log entry message with fmt.Sprintln instead of fmt.Sprint. Without this
// change, arguments are not separated by spaces.
type WswLogger struct{ s *zap.SugaredLogger }

func (f *WswLogger) DPanic(args ...interface{})                    { f.s.DPanicf(formatArgs(args)) }
func (f *WswLogger) DPanicf(template string, args ...interface{})  { f.s.DPanicf(template, args...) }
func (f *WswLogger) DPanicw(msg string, kvPairs ...interface{})    { f.s.DPanicw(msg, kvPairs...) }
func (f *WswLogger) Debug(args ...interface{})                     { f.s.Debugf(formatArgs(args)) }
func (f *WswLogger) Debugf(template string, args ...interface{})   { f.s.Debugf(template, args...) }
func (f *WswLogger) Debugw(msg string, kvPairs ...interface{})     { f.s.Debugw(msg, kvPairs...) }
func (f *WswLogger) Error(args ...interface{})                     { f.s.Errorf(formatArgs(args)) }
func (f *WswLogger) Errorf(template string, args ...interface{})   { f.s.Errorf(template, args...) }
func (f *WswLogger) Errorw(msg string, kvPairs ...interface{})     { f.s.Errorw(msg, kvPairs...) }
func (f *WswLogger) Fatal(args ...interface{})                     { f.s.Fatalf(formatArgs(args)) }
func (f *WswLogger) Fatalf(template string, args ...interface{})   { f.s.Fatalf(template, args...) }
func (f *WswLogger) Fatalw(msg string, kvPairs ...interface{})     { f.s.Fatalw(msg, kvPairs...) }
func (f *WswLogger) Info(args ...interface{})                      { f.s.Infof(formatArgs(args)) }
func (f *WswLogger) Infof(template string, args ...interface{})    { f.s.Infof(template, args...) }
func (f *WswLogger) Infow(msg string, kvPairs ...interface{})      { f.s.Infow(msg, kvPairs...) }
func (f *WswLogger) Panic(args ...interface{})                     { f.s.Panicf(formatArgs(args)) }
func (f *WswLogger) Panicf(template string, args ...interface{})   { f.s.Panicf(template, args...) }
func (f *WswLogger) Panicw(msg string, kvPairs ...interface{})     { f.s.Panicw(msg, kvPairs...) }
func (f *WswLogger) Warn(args ...interface{})                      { f.s.Warnf(formatArgs(args)) }
func (f *WswLogger) Warnf(template string, args ...interface{})    { f.s.Warnf(template, args...) }
func (f *WswLogger) Warnw(msg string, kvPairs ...interface{})      { f.s.Warnw(msg, kvPairs...) }
func (f *WswLogger) Warning(args ...interface{})                   { f.s.Warnf(formatArgs(args)) }
func (f *WswLogger) Warningf(template string, args ...interface{}) { f.s.Warnf(template, args...) }

// for backwards compatibility
func (f *WswLogger) Critical(args ...interface{})                   { f.s.Errorf(formatArgs(args)) }
func (f *WswLogger) Criticalf(template string, args ...interface{}) { f.s.Errorf(template, args...) }
func (f *WswLogger) Notice(args ...interface{})                     { f.s.Infof(formatArgs(args)) }
func (f *WswLogger) Noticef(template string, args ...interface{})   { f.s.Infof(template, args...) }

func (f *WswLogger) Named(name string) *WswLogger { return &WswLogger{s: f.s.Named(name)} }
func (f *WswLogger) Sync() error                     { return f.s.Sync() }
func (f *WswLogger) Zap() *zap.Logger                { return f.s.Desugar() }

func (f *WswLogger) IsEnabledFor(level zapcore.Level) bool {
	return f.s.Desugar().Core().Enabled(level)
}

func (f *WswLogger) With(args ...interface{}) *WswLogger {
	return &WswLogger{s: f.s.With(args...)}
}

func (f *WswLogger) WithOptions(opts ...zap.Option) *WswLogger {
	l := f.s.Desugar().WithOptions(opts...)
	return &WswLogger{s: l.Sugar()}
}

func formatArgs(args []interface{}) string { return strings.TrimSuffix(fmt.Sprintln(args...), "\n") }
