package parser

import (
	"errors"
	"testing"
)

func TestParse_Sent(t *testing.T) {
	line := `Apr 30 16:28:38 msa1 postfix/smtp[133]: 14080840640: to=<bob@ex228.warpmail.dev>, relay=ex228.warpmail.dev[133.125.70.228]:25, delay=0.14, delays=0.01/0.02/0.11/0, dsn=2.0.0, status=sent (250 2.0.0 Ok: queued as 33FDD8406D8)`
	r, err := Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Host != "msa1" {
		t.Errorf("Host = %q, want msa1", r.Host)
	}
	if r.Relay != "ex228.warpmail.dev" {
		t.Errorf("Relay = %q", r.Relay)
	}
	if r.Status != "sent" {
		t.Errorf("Status = %q", r.Status)
	}
	if r.DSN != "2.0.0" || r.DSNClass != "2" {
		t.Errorf("DSN = %q / class %q", r.DSN, r.DSNClass)
	}
	if r.SMTPCode != "250" {
		t.Errorf("SMTPCode = %q", r.SMTPCode)
	}
	if r.Delay != 0.14 {
		t.Errorf("Delay = %v", r.Delay)
	}
	want := [4]float64{0.01, 0.02, 0.11, 0}
	if r.Stages != want {
		t.Errorf("Stages = %v, want %v", r.Stages, want)
	}
}

func TestParse_DeferredEmbeddedCode(t *testing.T) {
	line := `Apr 30 16:28:38 msa1 postfix/smtp[137]: 170AC84065C: to=<bob@ex228.warpmail.dev>, relay=ex228.warpmail.dev[133.125.70.228]:25, delay=0.25, delays=0.03/0.18/0.03/0, dsn=4.7.0, status=deferred (host ex228.warpmail.dev[133.125.70.228] refused to talk to me: 421 4.7.0 mx1 Error: too many connections from 172.20.0.1)`
	r, err := Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != "deferred" {
		t.Errorf("Status = %q", r.Status)
	}
	if r.DSNClass != "4" {
		t.Errorf("DSNClass = %q", r.DSNClass)
	}
	if r.SMTPCode != "421" {
		t.Errorf("SMTPCode = %q", r.SMTPCode)
	}
}

func TestParse_BouncedFiveXX(t *testing.T) {
	line := `May 01 02:00:00 mta postfix/smtp[200]: ABC123: to=<x@example.com>, relay=mx.example.com[10.0.0.1]:25, delay=0.5, delays=0.1/0/0.2/0.2, dsn=5.1.1, status=bounced (host mx.example.com[10.0.0.1] said: 550 5.1.1 User unknown (in reply to RCPT TO command))`
	r, err := Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != "bounced" {
		t.Errorf("Status = %q", r.Status)
	}
	if r.DSNClass != "5" {
		t.Errorf("DSNClass = %q", r.DSNClass)
	}
	if r.SMTPCode != "550" {
		t.Errorf("SMTPCode = %q", r.SMTPCode)
	}
}

func TestParse_DeferredNoSMTPCode(t *testing.T) {
	line := `May 01 02:00:00 mta postfix/smtp[201]: ABC124: to=<x@example.com>, relay=mx.example.com[10.0.0.1]:25, delay=30, delays=0/0/30/0, dsn=4.4.1, status=deferred (connect to mx.example.com[10.0.0.1]:25: Connection timed out)`
	r, err := Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != "deferred" {
		t.Errorf("Status = %q", r.Status)
	}
	if r.SMTPCode != "" {
		t.Errorf("SMTPCode = %q, want empty", r.SMTPCode)
	}
}

func TestParse_RFC3339Timestamp(t *testing.T) {
	line := `2024-04-30T16:28:38.123Z msa1 postfix/smtp[133]: 14080840640: to=<bob@ex228.warpmail.dev>, relay=ex228.warpmail.dev[133.125.70.228]:25, delay=0.14, delays=0.01/0.02/0.11/0, dsn=2.0.0, status=sent (250 2.0.0 Ok)`
	r, err := Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Host != "msa1" {
		t.Errorf("Host = %q", r.Host)
	}
}

func TestParse_NotDelivery(t *testing.T) {
	cases := []string{
		`Apr 30 16:28:38 msa1 postfix/smtpd[124]: connect from unknown[172.20.0.1]`,
		`Apr 30 16:28:38 msa1 postfix/smtp[133]: warning: hostname does not resolve`,
		`Apr 30 16:28:38 msa1 postfix/master[1]: daemon started`,
		``,
	}
	for _, line := range cases {
		_, err := Parse(line)
		if !errors.Is(err, ErrNotDelivery) {
			t.Errorf("Parse(%q) err = %v, want ErrNotDelivery", line, err)
		}
	}
}

// nxadm/tail#25: partial lines are emitted indistinguishably from complete
// ones. Reject them so they do not produce metrics with truncated labels
// (e.g. node="ail-1" from "m" being lost, or status="def" from message cutoff).
func TestParse_PartialLine(t *testing.T) {
	cases := map[string]string{
		"prefix lost - host starts the line": `ail-1 postfix/smtp[133]: 14080840640: to=<bob@ex228.warpmail.dev>, relay=ex228.warpmail.dev[133.125.70.228]:25, delay=0.14, delays=0.01/0.02/0.11/0, dsn=2.0.0, status=sent (250 2.0.0 Ok: queued as 33FDD8406D8)`,
		"prefix lost - leading hyphen suffix": `-1 postfix/smtp[137]: 170AC84065C: to=<bob@ex228.warpmail.dev>, relay=ex228.warpmail.dev[133.125.70.228]:25, delay=195, delays=131/64/0/0, dsn=4.7.0, status=deferred (host ex228.warpmail.dev[133.125.70.228] refused to talk to me: 421 4.7.0 mx1 Error)`,
		"suffix lost - status word only":     `Apr 30 16:28:38 msa1 postfix/smtp[133]: 14080840640: to=<bob@ex228.warpmail.dev>, relay=ex228.warpmail.dev[133.125.70.228]:25, delay=0.14, delays=0.01/0.02/0.11/0, dsn=2.0.0, status=def`,
		"suffix lost - mid message":          `Apr 30 16:28:38 msa1 postfix/smtp[137]: 170AC84065C: to=<bob@ex228.warpmail.dev>, relay=ex228.warpmail.dev[133.125.70.228]:25, delay=195, delays=131/64/0/0, dsn=4.7.0, status=deferred (host ex228.warpmail.dev`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(line)
			if !errors.Is(err, ErrNotDelivery) {
				t.Errorf("Parse(%q) err = %v, want ErrNotDelivery", line, err)
			}
		})
	}
}
