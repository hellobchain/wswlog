/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"go.uber.org/zap/zapcore"
)

var (
	CheckedCountOpts = CounterOpts{
		Namespace:    "logging",
		Name:         "entries_checked",
		Help:         "Number of log entries checked against the active logging level",
		LabelNames:   []string{"level"},
		StatsdFormat: "%{#fqname}.%{level}",
	}

	WriteCountOpts = CounterOpts{
		Namespace:    "logging",
		Name:         "entries_written",
		Help:         "Number of log entries that are written",
		LabelNames:   []string{"level"},
		StatsdFormat: "%{#fqname}.%{level}",
	}
)

type Observer struct {
	CheckedCounter Counter
	WrittenCounter Counter
}

func NewObserver(provider Provider) *Observer {
	return &Observer{
		CheckedCounter: provider.NewCounter(CheckedCountOpts),
		WrittenCounter: provider.NewCounter(WriteCountOpts),
	}
}

func (m *Observer) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) {
	m.CheckedCounter.With("level", e.Level.String()).Add(1)
}

func (m *Observer) WriteEntry(e zapcore.Entry, fields []zapcore.Field) {
	m.WrittenCounter.With("level", e.Level.String()).Add(1)
}
