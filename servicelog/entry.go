package servicelog

import (
	"github.com/allegro/mesos-executor/runenv"
)

// Entry represents one scraped log line in flat key-value store.
type Entry map[string]interface{}

// Extender is type used to extend log data with additional data.
type Extender interface {
	// Extend returns a new log entry, based on the passed one, with additional
	// data. Original entry is not modified but duplicate keys are overwritten in
	// returned entry.
	Extend(Entry) Entry
}

// Extend returns a channel that will return log entries extended with passed
// extenders list. Original log entries are not modified, but duplicate keys are
// overwritten in returned ones.
func Extend(in <-chan Entry, extenders ...Extender) <-chan Entry {
	if len(extenders) == 0 {
		return in
	}
	out := make(chan Entry)
	go func() {
		for entry := range in {
			extendedEntry := entry
			for _, extender := range extenders {
				extendedEntry = extender.Extend(extendedEntry)
			}
			out <- extendedEntry
		}
	}()
	return out
}

// SystemDataExtender adds system specific data to passed log entry. It only
// adds data that is able to get.
type SystemDataExtender struct {
}

// Extend returns a new log entry, based on the passed entry, with hostname, region
// and availability zone added to it. Data is added only when it is possible to
// retrieve it from the system.
func (e SystemDataExtender) Extend(entry Entry) Entry {
	extendedEntry := Entry{}
	for key, value := range entry {
		extendedEntry[key] = value
	}
	e.setIfNoError(extendedEntry, "srchost", runenv.Hostname)
	e.setIfNoError(extendedEntry, "region", runenv.Region)
	e.setIfNoError(extendedEntry, "zone", runenv.AvailabilityZone)
	return extendedEntry
}

func (e SystemDataExtender) setIfNoError(entry Entry, key string, dataFunc func() (string, error)) {
	data, err := dataFunc()
	if err != nil {
		return
	}
	entry[key] = data
}

// StaticDataExtender adds data specified in the Data field to passed log entry.
type StaticDataExtender struct {
	Data map[string]interface{}
}

// Extend returns a new log entry, based on the passed entry, with Data map
// added to it.
func (e StaticDataExtender) Extend(entry Entry) Entry {
	extendedEntry := Entry{}
	for key, value := range entry {
		extendedEntry[key] = value
	}
	for key, value := range e.Data {
		extendedEntry[key] = value
	}
	return extendedEntry
}
