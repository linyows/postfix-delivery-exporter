package tailer

import (
	"context"
	"io"
	"log/slog"
	"sync"

	"github.com/nxadm/tail"
)

// Tailer follows one or more files and emits each new line on a channel.
// File rotation (rename, copytruncate) is handled via nxadm/tail's ReOpen.
type Tailer struct {
	files         []string
	fromBeginning bool
	logger        *slog.Logger
}

// New constructs a Tailer. If fromBeginning is true, existing content is read
// before tailing; otherwise tailing starts from the current end of file.
func New(files []string, fromBeginning bool, logger *slog.Logger) *Tailer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Tailer{files: files, fromBeginning: fromBeginning, logger: logger}
}

// Run tails the configured files until ctx is canceled, sending each line on out.
// out is not closed; the caller owns it.
func (t *Tailer) Run(ctx context.Context, out chan<- string) error {
	var loc *tail.SeekInfo
	if !t.fromBeginning {
		loc = &tail.SeekInfo{Whence: io.SeekEnd}
	}
	cfg := tail.Config{
		ReOpen:    true,
		Follow:    true,
		MustExist: false,
		Location:  loc,
		Logger:    tail.DiscardingLogger,
	}

	var wg sync.WaitGroup
	tails := make([]*tail.Tail, 0, len(t.files))
	for _, path := range t.files {
		tt, err := tail.TailFile(path, cfg)
		if err != nil {
			t.logger.Error("failed to tail file", "path", path, "err", err)
			continue
		}
		tails = append(tails, tt)
		wg.Add(1)
		go func(path string, tt *tail.Tail) {
			defer wg.Done()
			for line := range tt.Lines {
				if line.Err != nil {
					t.logger.Warn("tail error", "path", path, "err", line.Err)
					continue
				}
				select {
				case out <- line.Text:
				case <-ctx.Done():
					return
				}
			}
		}(path, tt)
	}

	<-ctx.Done()
	for _, tt := range tails {
		_ = tt.Stop()
	}
	wg.Wait()
	return nil
}
