package core

import (
	"io"
	"log/slog"
	"sync"
	"time"
)

// ReportQueue writes keyboard reports to the host in order on its own
// goroutine, so a slow host never holds up audio processing.
type ReportQueue struct {
	w     io.Writer
	delay time.Duration
	log   *slog.Logger
	ch    chan []Report
	wg    sync.WaitGroup

	// OnBatch, if set before the first Send, is called after each batch
	// has been written.
	OnBatch func()
}

// NewReportQueue starts a queue writing to w, pausing delay after each
// report. log may be nil.
func NewReportQueue(w io.Writer, delay time.Duration, log *slog.Logger) *ReportQueue {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	q := &ReportQueue{w: w, delay: delay, log: log, ch: make(chan []Report, 1024)}
	q.wg.Add(1)
	go q.run()
	return q
}

// Send queues reports for writing. If the host has stopped taking input and
// the queue is full, the reports are dropped rather than stalling the pad.
func (q *ReportQueue) Send(reports []Report) {
	select {
	case q.ch <- reports:
	default:
		q.log.Error("the host is not accepting keys; dropping text")
	}
}

// Close writes what is queued, lifts all keys, and stops the queue.
func (q *ReportQueue) Close() {
	q.ch <- []Report{releaseAll}
	close(q.ch)
	q.wg.Wait()
}

func (q *ReportQueue) run() {
	defer q.wg.Done()
	for batch := range q.ch {
		for _, r := range batch {
			if _, err := q.w.Write(r[:]); err != nil {
				q.log.Error("write to host failed; dropping the rest of this text", "err", err)
				break
			}
			if q.delay > 0 {
				time.Sleep(q.delay)
			}
		}
		if q.OnBatch != nil {
			q.OnBatch()
		}
	}
}
