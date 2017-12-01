package appender

import "github.com/allegro/mesos-executor/servicelog"

// Appender delivers service log entries to their destination.
type Appender interface {
	Append(entries <-chan servicelog.Entry)
}
