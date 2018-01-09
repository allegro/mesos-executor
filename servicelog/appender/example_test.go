package appender_test

import (
	"log"
	"net"

	"github.com/allegro/mesos-executor/servicelog/appender"
)

func ExampleNewLogstash() {
	// create writer that will be used to send logs
	writer, err := net.DialUDP("udp", nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	// create appender with desired options
	_, err = appender.NewLogstash(writer,
		appender.LogstashRateLimit(100),
		appender.LogstashSizeLimit(16000))
	if err != nil {
		log.Fatal(err)
	}
}
