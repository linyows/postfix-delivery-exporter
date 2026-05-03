package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/linyows/postfix-delivery-exporter/internal/collector"
	"github.com/linyows/postfix-delivery-exporter/internal/parser"
	"github.com/linyows/postfix-delivery-exporter/internal/tailer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	var (
		listen        string
		metricsPath   string
		filesArg      string
		allowlistArg  string
		fromBeginning bool
		stdin         bool
	)
	flag.StringVar(&listen, "web.listen-address", ":9620", "Address to listen on for /metrics.")
	flag.StringVar(&metricsPath, "web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	flag.StringVar(&filesArg, "log.files", "", "Comma-separated list of postfix log files to tail.")
	flag.StringVar(&allowlistArg, "relay.allowlist", "", "Comma-separated relay hostnames to keep as labels; others become \"other\". Empty = pass through all.")
	flag.BoolVar(&fromBeginning, "log.from-beginning", false, "Read each file from the beginning instead of the current end.")
	flag.BoolVar(&stdin, "log.stdin", false, "Read log lines from stdin (for replay/testing). Mutually exclusive with -log.files.")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if !stdin && filesArg == "" {
		logger.Error("either -log.files or -log.stdin is required")
		os.Exit(2)
	}
	if stdin && filesArg != "" {
		logger.Error("-log.stdin and -log.files are mutually exclusive")
		os.Exit(2)
	}

	reg := prometheus.NewRegistry()
	c := collector.New(reg, collector.Options{RelayAllowlist: splitCSV(allowlistArg)})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	lines := make(chan string, 1024)
	go consume(ctx, lines, c, logger)

	go func() {
		if stdin {
			scanStdin(ctx, lines, logger)
		} else {
			files := splitCSV(filesArg)
			t := tailer.New(files, fromBeginning, logger)
			if err := t.Run(ctx, lines); err != nil {
				logger.Error("tailer stopped", "err", err)
			}
		}
	}()

	mux := http.NewServeMux()
	mux.Handle(metricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("postfix-delivery-exporter\n" + metricsPath + "\n"))
	})
	srv := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logger.Info("listening", "addr", listen, "path", metricsPath)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("http server error", "err", err)
		os.Exit(1)
	}
}

func consume(ctx context.Context, lines <-chan string, c *collector.Collector, logger *slog.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-lines:
			if !ok {
				return
			}
			rec, err := parser.Parse(line)
			if err != nil {
				continue
			}
			if rec.Status == "" {
				c.IncParseError()
				continue
			}
			c.Observe(rec)
		}
	}
}

func scanStdin(ctx context.Context, out chan<- string, logger *slog.Logger) {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		select {
		case out <- sc.Text():
		case <-ctx.Done():
			return
		}
	}
	if err := sc.Err(); err != nil {
		logger.Error("stdin scan error", "err", err)
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
