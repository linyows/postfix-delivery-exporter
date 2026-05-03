package collector

import (
	"github.com/linyows/postfix-delivery-exporter/internal/parser"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "postfix"
	subsystem = "delivery"

	// labelOther is the sentinel applied to relays that are not in the allowlist.
	labelOther = "other"

	stageBeforeQmgr   = "before_qmgr"
	stageInQmgr       = "in_qmgr"
	stageConnSetup    = "conn_setup"
	stageTransmission = "transmission"
)

var stageNames = [4]string{stageBeforeQmgr, stageInQmgr, stageConnSetup, stageTransmission}

// Collector turns parsed delivery records into Prometheus metrics.
type Collector struct {
	deliveryTotal *prometheus.CounterVec
	duration      *prometheus.HistogramVec
	stage         *prometheus.HistogramVec
	parseErrors   prometheus.Counter
	allowlist     map[string]struct{}
}

// Options configures collector behavior.
type Options struct {
	// RelayAllowlist, when non-empty, restricts the relay label to listed
	// hostnames; other relays are reported as "other".
	RelayAllowlist []string
}

// New constructs a Collector and registers its metrics with reg.
func New(reg prometheus.Registerer, opts Options) *Collector {
	// delay= can range from sub-second (sent) to hours (deferred messages
	// that have been retrying), so cover both regimes with explicit buckets.
	durationBuckets := []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300, 1800, 7200, 28800, 86400}
	// Per-stage values within a single attempt are bounded by the smtp
	// connect/data timeout (Postfix default 5 min); a 10-min cap is plenty.
	stageBuckets := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300, 600}
	c := &Collector{
		deliveryTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "total",
			Help:      "Total number of delivery attempts observed in postfix/smtp logs.",
		}, []string{"node", "relay", "status", "dsn_class", "smtp_code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "duration_seconds",
			Help:      "Total time the message had been in postfix at the moment this attempt was logged (delay= field). For status=sent this is end-to-end delivery time; for status=deferred/bounced it includes prior queue wait, so values can reach hours.",
			Buckets:   durationBuckets,
		}, []string{"node", "relay", "status"}),
		stage: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "stage_seconds",
			Help:      "Per-stage time within one delivery attempt from the delays=a/b/c/d field. Stages: before_qmgr, in_qmgr (includes queue wait between retries), conn_setup, transmission. conn_setup + transmission approximates SMTP response time.",
			Buckets:   stageBuckets,
		}, []string{"node", "relay", "stage"}),
		parseErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "parse_errors_total",
			Help:      "Number of log lines that looked like delivery records but failed to parse.",
		}),
	}
	if len(opts.RelayAllowlist) > 0 {
		c.allowlist = make(map[string]struct{}, len(opts.RelayAllowlist))
		for _, r := range opts.RelayAllowlist {
			if r != "" {
				c.allowlist[r] = struct{}{}
			}
		}
	}
	reg.MustRegister(c.deliveryTotal, c.duration, c.stage, c.parseErrors)
	return c
}

// Observe records a single parsed delivery record.
func (c *Collector) Observe(rec *parser.Record) {
	relay := c.relayLabel(rec.Relay)
	c.deliveryTotal.WithLabelValues(rec.Host, relay, rec.Status, rec.DSNClass, rec.SMTPCode).Inc()
	c.duration.WithLabelValues(rec.Host, relay, rec.Status).Observe(rec.Delay)
	for i, v := range rec.Stages {
		c.stage.WithLabelValues(rec.Host, relay, stageNames[i]).Observe(v)
	}
}

// IncParseError records a parse failure on a line that was expected to be a delivery record.
func (c *Collector) IncParseError() { c.parseErrors.Inc() }

func (c *Collector) relayLabel(relay string) string {
	if c.allowlist == nil {
		return relay
	}
	if _, ok := c.allowlist[relay]; ok {
		return relay
	}
	return labelOther
}
