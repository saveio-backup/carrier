/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-09-12
 */
package network

import (
	"log"
	"os"
	"time"

	"github.com/rcrowley/go-metrics"
)

type Metric struct {
	Gauge   metrics.Gauge
	Counter metrics.Counter
	Meter   metrics.Meter
}

func initMetric() Metric {
	go metrics.Log(metrics.DefaultRegistry, 5*time.Second, log.New(os.Stderr, "metrics: ", log.Lmicroseconds))
	return Metric{
		Gauge:   metrics.NewGauge(),
		Counter: metrics.NewCounter(),
		Meter:   metrics.NewMeter(),
	}
}
