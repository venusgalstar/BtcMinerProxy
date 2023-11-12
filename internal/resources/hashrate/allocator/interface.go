package allocator

import (
	"context"
	"net/url"
	"time"

	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/proxy"
)

type StratumProxyInterface interface {
	Connect(ctx context.Context) error
	Run(ctx context.Context) error
	SetDest(ctx context.Context, dest *url.URL, onSubmit func(diff float64)) error

	GetID() string
	GetHashrate() proxy.Hashrate
	GetDifficulty() float64
	GetDest() *url.URL
	GetSourceWorkerName() string
	GetDestWorkerName() string
	GetMinerConnectedAt() time.Time
	GetStats() map[string]int
	GetDestConns() *map[string]string
}
