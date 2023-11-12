package hashrate

import (
	"time"
)

type Counter interface {
	Add(v float64)
	Value() float64
	ValuePer(t time.Duration) float64
}

type HashrateFactory = func() *Hashrate
