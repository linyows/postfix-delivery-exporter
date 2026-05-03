package parser

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// ErrNotDelivery is returned for log lines that are not postfix/smtp delivery results.
var ErrNotDelivery = errors.New("not a delivery line")

// Record holds the fields extracted from a single postfix/smtp delivery log line.
type Record struct {
	Host     string
	Relay    string
	Status   string
	DSN      string
	DSNClass string
	SMTPCode string
	Delay    float64
	Stages   [4]float64
}

var (
	headerRE = regexp.MustCompile(`(\S+)\s+postfix/smtp\[\d+\]:\s+\S+:\s+(.*)$`)
	relayRE  = regexp.MustCompile(`relay=([^,\[\s]+)`)
	delayRE  = regexp.MustCompile(`\bdelay=([\d.]+)`)
	delaysRE = regexp.MustCompile(`\bdelays=([\d.]+)/([\d.]+)/([\d.]+)/([\d.]+)`)
	dsnRE    = regexp.MustCompile(`\bdsn=(\d)\.(\d+)\.(\d+)`)
	statusRE = regexp.MustCompile(`\bstatus=(\w+)(?:\s+\((.*)\))?\s*$`)
	codeRE   = regexp.MustCompile(`\b([245]\d{2})\s+\d+\.\d+\.\d+`)
)

// Parse extracts a Record from a single syslog line. Lines that are not
// postfix/smtp delivery results return ErrNotDelivery.
func Parse(line string) (*Record, error) {
	line = strings.TrimRight(line, "\r\n")
	m := headerRE.FindStringSubmatch(line)
	if m == nil {
		return nil, ErrNotDelivery
	}
	rest := m[2]
	statusMatch := statusRE.FindStringSubmatch(rest)
	if statusMatch == nil {
		return nil, ErrNotDelivery
	}
	rec := &Record{Host: m[1], Status: statusMatch[1]}
	if mm := relayRE.FindStringSubmatch(rest); mm != nil {
		rec.Relay = mm[1]
	}
	if mm := delayRE.FindStringSubmatch(rest); mm != nil {
		rec.Delay, _ = strconv.ParseFloat(mm[1], 64)
	}
	if mm := delaysRE.FindStringSubmatch(rest); mm != nil {
		for i := range 4 {
			rec.Stages[i], _ = strconv.ParseFloat(mm[i+1], 64)
		}
	}
	if mm := dsnRE.FindStringSubmatch(rest); mm != nil {
		rec.DSNClass = mm[1]
		rec.DSN = mm[1] + "." + mm[2] + "." + mm[3]
	}
	if statusMatch[2] != "" {
		if cm := codeRE.FindStringSubmatch(statusMatch[2]); cm != nil {
			rec.SMTPCode = cm[1]
		}
	}
	return rec, nil
}
