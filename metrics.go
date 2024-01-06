package healthcheck

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"reflect"
)

const (
	metricsNameSpase = "healthcheck"
	metricsSubsystem = "metrics"
	metricsHelp      = "Checking available %s"
	metricsHelpDur   = "Checking duration %s"
	HandlerMetrics   = "/metrics"
)

var (
	// Metrics interface implementation check fot struct metrics
	_ Metrics = (*metrics)(nil)
)

// Metrics - Interface for metrics access and mock it
type Metrics interface {
	HandlerMetrics() http.Handler
	Register(name string) error
	Save(name string, t float64, err error) error
}

// metrics - хранения данных для взаимодействия с метриками
type metrics struct {
	// mm - collectors storage for access
	mm map[string]prometheus.Collector

	registry *prometheus.Registry
}

func NewMetrics(buildInfo, goCollector, processCollector bool) *metrics {

	m := &metrics{
		mm:       make(map[string]prometheus.Collector),
		registry: prometheus.NewRegistry(),
	}

	if buildInfo {
		m.registry.MustRegister(
			collectors.NewBuildInfoCollector(),
		)
	}

	if goCollector {
		m.registry.MustRegister(
			collectors.NewGoCollector(),
		)
	}

	if processCollector {
		m.registry.MustRegister(
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{ReportErrors: true}),
		)
	}

	return m

}

// Register - register new metrics collector
func (m *metrics) Register(name string) error {

	var err error

	m.mm[name] = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   metricsNameSpase,
		Subsystem:   metricsSubsystem,
		Name:        name,
		Help:        fmt.Sprintf(metricsHelp, name),
		ConstLabels: nil,
	})

	if err = m.registry.Register(m.mm[name]); err != nil {
		return fmt.Errorf("collector registration [%s]: %v", name, err)
	}

	m.mm[fmt.Sprintf("%s_dur", name)] = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   metricsNameSpase,
		Subsystem:   metricsSubsystem,
		Name:        fmt.Sprintf("%s_dur", name),
		Help:        fmt.Sprintf(metricsHelpDur, name),
		ConstLabels: nil,
	})

	if err = m.registry.Register(m.mm[fmt.Sprintf("%s_dur", name)]); err != nil {
		return fmt.Errorf("collector registration [%s]: %v", fmt.Sprintf("%s_dur", name), err)
	}

	return nil

}

// Save - set collector with name current value to v
func (m *metrics) Save(name string, t float64, err error) error {

	var (
		ok        bool
		collector prometheus.Collector
	)

	if collector, ok = m.mm[name]; !ok {
		return fmt.Errorf("gauge collector with name %s - not found", name)
	}

	switch collector.(type) {
	case prometheus.Gauge:
		{
			switch err {
			case nil:
				m.mm[name].(prometheus.Gauge).Set(1)
			default:
				m.mm[name].(prometheus.Gauge).Set(0)
			}

		}
	default:
		return fmt.Errorf("save metric to collector as type Gauge %s, but collector type is %v", name, reflect.TypeOf(m.mm[name]).String())
	}

	if collector, ok = m.mm[fmt.Sprintf("%s_dur", name)]; !ok {
		return fmt.Errorf("рistogram collector with name %s - not found", name)
	}

	switch collector.(type) {
	case prometheus.Histogram:
		{
			m.mm[fmt.Sprintf("%s_dur", name)].(prometheus.Histogram).Observe(t)
		}
	default:
		return fmt.Errorf("save metric to collector as type Histogram %s, but collector type is %v", name, reflect.TypeOf(m.mm[name]).String())
	}

	return nil

}

// HandlerMetrics - handler for incoming requests on endpoint like /metrics
func (m *metrics) HandlerMetrics() http.Handler {

	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry})

}
