package xio_test

import (
	"fmt"
	"os"

	"github.com/allegro/mesos-executor/xio"
)

func ExampleDecorateWriter() {
	writer := xio.DecorateWriter(os.Stdout,
		xio.SizeLimit(1),
		xio.RateLimit(1))

	_, err := writer.Write([]byte("bytes"))

	fmt.Println(err)

	// Output:
	// limit of written bytes exceeded
}
