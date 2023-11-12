package contractmanager

import (
	"time"

	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate"
)

type TermsCommon interface {
	GetID() string
	GetState() hashrate.BlockchainState
	GetSeller() string
	GetBuyer() string
	GetStartsAt() *time.Time
	GetDuration() time.Duration
	GetHashrateGHS() float64
}
