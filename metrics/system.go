package metrics

import (
	"fmt"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	metrics "github.com/rcrowley/go-metrics"
	"github.com/shirou/gopsutil/process"
)

// CPUTime returns an amount of CPU time (in seconds) assigned to executor process
// by system.
func CPUTime() (float64, error) {
	p := getExecutorProcess()
	t, err := p.Times()
	if err != nil {
		return 0, fmt.Errorf("Unable to get CPU times: %s", err)
	}

	// We cannot use cpu.TimesStat struct Total function because it sums all values -
	// and we do not want to add cpu idle value to total.
	total := t.User + t.System + t.Nice + t.Iowait + t.Irq + t.Softirq +
		t.Steal + t.Guest + t.GuestNice + t.Stolen

	return total, nil
}

// CaptureCPUTime starts collecting cpu time of the executor process with given
// interval. It is a blocking call so it is advised to call this function in
// goroutine.
func CaptureCPUTime(interval time.Duration) {
	cpuUtilizationGauge := metrics.NewGaugeFloat64()
	err := metrics.Register("runtime.CpuStats.Utilization", cpuUtilizationGauge)
	if err != nil {
		log.Warnf("Could not register CPU utilisation metric: %s", err)
		return
	}
	ticker := time.NewTicker(interval)

	lastSeconds, _ := CPUTime() // if we are unable to get initial value 0.0 is okay
	for range ticker.C {
		seconds, err := CPUTime()
		if err != nil {
			log.WithError(err).Warn("Unable to send current cpu utilization metric")
			continue
		}
		cpuUtilization := (seconds - lastSeconds) / interval.Seconds()
		cpuUtilizationGauge.Update(cpuUtilization)
		lastSeconds = seconds
	}
}

func getExecutorProcess() *process.Process {
	pid := os.Getpid()
	p, _ := process.NewProcess(int32(pid)) // never fails as we use our own pid here
	return p
}
