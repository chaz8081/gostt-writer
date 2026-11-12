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

// Send queues reports for writing.
func (q *ReportQueue) Send(reports []Report) { q.ch <- reports }

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
	}
}
