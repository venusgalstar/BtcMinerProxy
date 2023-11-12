package hashrate

import (
	"sync/atomic"
	"time"
)

type Mean struct {
	totalWork       *atomic.Uint64
	firstSubmitTime *atomic.Int64 // stores first submit time in unix seconds
	lastSubmitTime  *atomic.Int64 // stores last submit time in unix seconds
}

// NewMean creates a new Mean hashrate counter, which adds all submitted work and divides it by the total duration
// it is also used to track the first and last submit time and total work
func NewMean() *Mean {
	return &Mean{
		totalWork:       &atomic.Uint64{},
		firstSubmitTime: &atomic.Int64{},
		lastSubmitTime:  &atomic.Int64{},
	}
}

func (h *Mean) Add(diff float64) {
	h.totalWork.Add(uint64(diff))

	now := time.Now()
	h.maybeSetFirstSubmitTime(now)
	h.setLastSubmitTime(now)
}

func (h *Mean) Value() float64 {
	return float64(h.totalWork.Load())
}

func (h *Mean) ValuePer(t time.Duration) float64 {
	totalDuration := h.GetTotalDuration()
	if totalDuration == 0 {
		return 0
	}
	return float64(h.GetTotalWork()) / float64(totalDuration/t)
}

func (h *Mean) GetLastSubmitTime() time.Time {
	return time.Unix(h.lastSubmitTime.Load(), 0)
}

func (h *Mean) GetTotalWork() uint64 {
	return h.totalWork.Load()
}

func (h *Mean) GetTotalDuration() time.Duration {
	durationSeconds := time.Now().Unix() - h.firstSubmitTime.Load()
	return time.Duration(durationSeconds) * time.Second
}

func (h *Mean) maybeSetFirstSubmitTime(t time.Time) {
	h.firstSubmitTime.CompareAndSwap(0, t.Unix())
}

func (h *Mean) setLastSubmitTime(t time.Time) {
	h.lastSubmitTime.Store(t.Unix())
}
