package collector

import (
	"strings"
	"testing"

	"github.com/linyows/postfix-delivery-exporter/internal/parser"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserve_Counts(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := New(reg, Options{})
	c.Observe(&parser.Record{
		Host: "msa1", Relay: "mx.example.com", Status: "sent",
		DSN: "2.0.0", DSNClass: "2", SMTPCode: "250",
		Delay: 0.2, Stages: [4]float64{0.01, 0.02, 0.15, 0.02},
	})
	c.Observe(&parser.Record{
		Host: "msa1", Relay: "mx.example.com", Status: "deferred",
		DSN: "4.7.0", DSNClass: "4", SMTPCode: "421",
		Delay: 0.3, Stages: [4]float64{0.05, 0.2, 0.05, 0},
	})

	expected := `
# HELP postfix_delivery_total Total number of delivery attempts observed in postfix/smtp logs.
# TYPE postfix_delivery_total counter
postfix_delivery_total{dsn_class="2",node="msa1",relay="mx.example.com",smtp_code="250",status="sent"} 1
postfix_delivery_total{dsn_class="4",node="msa1",relay="mx.example.com",smtp_code="421",status="deferred"} 1
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "postfix_delivery_total"); err != nil {
		t.Error(err)
	}
}

func TestObserve_AllowlistFolding(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := New(reg, Options{RelayAllowlist: []string{"mx.example.com"}})
	c.Observe(&parser.Record{Host: "h", Relay: "mx.example.com", Status: "sent", DSNClass: "2", SMTPCode: "250"})
	c.Observe(&parser.Record{Host: "h", Relay: "mx.other.com", Status: "sent", DSNClass: "2", SMTPCode: "250"})
	c.Observe(&parser.Record{Host: "h", Relay: "mx.another.com", Status: "sent", DSNClass: "2", SMTPCode: "250"})

	expected := `
# HELP postfix_delivery_total Total number of delivery attempts observed in postfix/smtp logs.
# TYPE postfix_delivery_total counter
postfix_delivery_total{dsn_class="2",node="h",relay="mx.example.com",smtp_code="250",status="sent"} 1
postfix_delivery_total{dsn_class="2",node="h",relay="other",smtp_code="250",status="sent"} 2
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "postfix_delivery_total"); err != nil {
		t.Error(err)
	}
}
