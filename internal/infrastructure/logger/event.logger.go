package logger

import (
	"time"

	"github.com/rs/zerolog"
)

type Event struct {
	ev *zerolog.Event
}

func (e *Event) Str(key, val string) *Event {
	e.ev = e.ev.Str(key, val)
	return e
}

func (e *Event) Int(key string, val int) *Event {
	e.ev = e.ev.Int(key, val)
	return e
}

func (e *Event) Int64(key string, val int64) *Event {
	e.ev = e.ev.Int64(key, val)
	return e
}

func (e *Event) Uint64(key string, val uint64) *Event {
	e.ev = e.ev.Uint64(key, val)
	return e
}

func (e *Event) Float64(key string, val float64) *Event {
	e.ev = e.ev.Float64(key, val)
	return e
}

func (e *Event) Bool(key string, val bool) *Event {
	e.ev = e.ev.Bool(key, val)
	return e
}

func (e *Event) Dur(key string, val time.Duration) *Event {
	e.ev = e.ev.Dur(key, val)
	return e
}

func (e *Event) Time(key string, val time.Time) *Event {
	e.ev = e.ev.Time(key, val)
	return e
}

func (e *Event) Err(err error) *Event {
	e.ev = e.ev.Err(err)
	return e
}

func (e *Event) Interface(key string, val interface{}) *Event {
	e.ev = e.ev.Interface(key, val)
	return e
}

func (e *Event) Msg(msg string) {
	e.ev.Msg(msg)
}

func (e *Event) Msgf(format string, args ...interface{}) {
	e.ev.Msgf(format, args...)
}

func (e *Event) Send() {
	e.ev.Send()
}
