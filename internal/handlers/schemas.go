package handlers

import "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/allocator"

type MinersResponse struct {
	TotalHashrateGHS     int
	UsedHashrateGHS      int
	AvailableHashrateGHS int

	TotalMiners   int
	BusyMiners    int
	FreeMiners    int
	VettingMiners int

	Miners []Miner
}

type Miner struct {
	Resource

	ID                    string
	WorkerName            string
	Status                string
	HashrateAvgGHS        map[string]int
	CurrentDestination    string
	CurrentDifficulty     int
	ConnectedAt           string
	Uptime                string
	ActivePoolConnections *map[string]string `json:",omitempty"`
	Destinations          []*allocator.DestItem
	Stats                 interface{}
}

type Contract struct {
	Resource

	Role                    string
	Stage                   string
	ID                      string
	BuyerAddr               string
	SellerAddr              string
	ResourceEstimatesTarget map[string]int
	ResourceEstimatesActual map[string]int

	StartTimestamp    *string
	EndTimestamp      *string
	Duration          string
	Elapsed           *string
	ApplicationStatus string
	BlockchainStatus  string
	Dest              string
	Miners            []*allocator.DestItem
}

type Resource struct {
	Self string
}

type Worker struct {
	WorkerName string
	Hashrate   map[string]float64
}
