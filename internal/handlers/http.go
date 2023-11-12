package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/config"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/contractmanager"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/allocator"
	hr "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/hashrate"
	"golang.org/x/exp/slices"
)

type Proxy interface {
	SetDest(ctx context.Context, newDestURL *url.URL, onSubmit func(diff float64)) error
}

type ContractFactory func(contractData *hashrate.Terms) (resources.Contract, error)

type HTTPHandler struct {
	globalHashrate         *hr.GlobalHashrate
	allocator              *allocator.Allocator
	contractManager        *contractmanager.ContractManager
	hashrateCounterDefault string
	publicUrl              *url.URL
	pubKey                 string
	log                    interfaces.ILogger
}

func NewHTTPHandler(allocator *allocator.Allocator, contractManager *contractmanager.ContractManager, globalHashrate *hr.GlobalHashrate, publicUrl *url.URL, hashrateCounter string, log interfaces.ILogger) *gin.Engine {
	handl := &HTTPHandler{
		allocator:              allocator,
		contractManager:        contractManager,
		globalHashrate:         globalHashrate,
		publicUrl:              publicUrl,
		hashrateCounterDefault: hashrateCounter,
		log:                    log,
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.GET("/healthcheck", handl.HealthCheck)
	r.GET("/miners", handl.GetMiners)
	r.GET("/contracts", handl.GetContracts)
	r.GET("/workers", handl.GetWorkers)

	r.POST("/change-dest", handl.ChangeDest)
	r.POST("/contracts", handl.CreateContract)

	err := r.SetTrustedProxies(nil)
	if err != nil {
		panic(err)
	}

	return r
}

func (h *HTTPHandler) HealthCheck(ctx *gin.Context) {
	ctx.JSON(200, gin.H{
		"status":  "healthy",
		"version": config.BuildVersion,
	})
}

func (h *HTTPHandler) ChangeDest(ctx *gin.Context) {
	urlString := ctx.Query("dest")
	if urlString == "" {
		ctx.JSON(400, gin.H{"error": "empty destination"})
		return
	}
	dest, err := url.Parse(urlString)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	miners := h.allocator.GetMiners()
	miners.Range(func(m *allocator.Scheduler) bool {
		m.SetPrimaryDest(dest)
		return true
	})

	ctx.JSON(200, gin.H{"status": "ok"})
}

func (h *HTTPHandler) CreateContract(ctx *gin.Context) {
	dest, err := url.Parse(ctx.Query("dest"))
	if err != nil {
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}
	hrGHS, err := strconv.ParseInt(ctx.Query("hrGHS"), 10, 0)
	if err != nil {
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}
	duration, err := time.ParseDuration(ctx.Query("duration"))
	if err != nil {
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}
	now := time.Now()
	destEnc, err := lib.EncryptString(dest.String(), h.pubKey)
	if err != nil {
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	h.contractManager.AddContract(context.Background(), &hashrate.EncryptedTerms{
		Base: hashrate.Base{
			ContractID: lib.GetRandomAddr().String(),
			Seller:     "",
			Buyer:      "",
			StartsAt:   &now,
			Duration:   duration,
			Hashrate:   float64(hrGHS) * 1e9,
		},
		DestEncrypted: destEnc,
	})

	ctx.JSON(200, gin.H{"status": "ok"})
}

func (c *HTTPHandler) GetContracts(ctx *gin.Context) {
	data := []Contract{}
	c.contractManager.GetContracts().Range(func(item resources.Contract) bool {
		contract := c.mapContract(item)
		// 		contract.Miners = miners
		data = append(data, *contract)
		return true
	})

	slices.SortStableFunc(data, func(a Contract, b Contract) bool {
		return a.ID < b.ID
	})

	ctx.JSON(200, data)
}

func (c *HTTPHandler) GetMiners(ctx *gin.Context) {
	Miners := []Miner{}

	var (
		TotalHashrateGHS float64
		UsedHashrateGHS  float64

		TotalMiners   int
		BusyMiners    int
		FreeMiners    int
		VettingMiners int
	)

	c.allocator.GetMiners().Range(func(m *allocator.Scheduler) bool {
		// _, usedHR := mapDestItems(m.GetCurrentDestSplit(), m.GetHashRateGHS())
		// UsedHashrateGHS += usedHR

		hrGHS, ok := m.GetHashrate().GetHashrateAvgGHSCustom(c.hashrateCounterDefault)
		if !ok {
			c.log.DPanicf("hashrate counter not found, %s", c.hashrateCounterDefault)
		}
		TotalHashrateGHS += hrGHS

		TotalMiners += 1

		switch m.GetStatus() {
		case allocator.MinerStatusFree:
			FreeMiners += 1
		case allocator.MinerStatusVetting:
			VettingMiners += 1
		case allocator.MinerStatusBusy:
			BusyMiners += 1
		}

		miner := c.MapMiner(m)
		Miners = append(Miners, *miner)

		return true
	})

	slices.SortStableFunc(Miners, func(a Miner, b Miner) bool {
		return a.ID < b.ID
	})

	res := &MinersResponse{
		TotalMiners:   TotalMiners,
		BusyMiners:    BusyMiners,
		FreeMiners:    FreeMiners,
		VettingMiners: VettingMiners,

		TotalHashrateGHS:     int(TotalHashrateGHS),
		AvailableHashrateGHS: int(TotalHashrateGHS - UsedHashrateGHS),
		UsedHashrateGHS:      int(UsedHashrateGHS),

		Miners: Miners,
	}

	ctx.JSON(200, res)
}

func (c *HTTPHandler) MapMiner(m *allocator.Scheduler) *Miner {
	// destItems, _ := mapDestItems(m.GetCurrentDestSplit(), m.GetHashRateGHS())
	// SMA9m, _ := hashrate.GetHashrateAvgGHSCustom(0)

	return &Miner{
		Resource: Resource{
			Self: c.publicUrl.JoinPath(fmt.Sprintf("/miners/%s", m.GetID())).String(),
		},
		ID:                    m.GetID(),
		WorkerName:            m.GetWorkerName(),
		Status:                m.GetStatus().String(),
		CurrentDifficulty:     int(m.GetCurrentDifficulty()),
		HashrateAvgGHS:        mapHRToInt(m),
		CurrentDestination:    m.GetCurrentDest().String(),
		ConnectedAt:           m.GetConnectedAt().Format(time.RFC3339),
		Stats:                 m.GetStats(),
		Uptime:                formatDuration(m.GetUptime()),
		ActivePoolConnections: m.GetDestConns(),
		Destinations:          m.GetDestinations(),
	}
}

func (c *HTTPHandler) GetWorkers(ctx *gin.Context) {
	workers := []*Worker{}

	c.globalHashrate.Range(func(item *hr.WorkerHashrateModel) bool {
		worker := &Worker{
			WorkerName: item.ID,
			Hashrate:   item.GetHashrateAvgGHSAll(),
		}
		workers = append(workers, worker)
		return true
	})

	slices.SortStableFunc(workers, func(a *Worker, b *Worker) bool {
		return a.WorkerName < b.WorkerName
	})

	ctx.JSON(200, workers)
}

func (p *HTTPHandler) mapContract(item resources.Contract) *Contract {
	return &Contract{
		Resource: Resource{
			Self: p.publicUrl.JoinPath(fmt.Sprintf("/contracts/%s", item.GetID())).String(),
		},
		Role:                    item.GetRole().String(),
		Stage:                   item.GetValidationStage().String(),
		ID:                      item.GetID(),
		BuyerAddr:               item.GetBuyer(),
		SellerAddr:              item.GetSeller(),
		ResourceEstimatesTarget: roundResourceEstimates(item.GetResourceEstimates()),
		ResourceEstimatesActual: roundResourceEstimates(item.GetResourceEstimatesActual()),
		Duration:                formatDuration(item.GetDuration()),
		StartTimestamp:          TimePtrToStringPtr(item.GetStartedAt()),
		EndTimestamp:            TimePtrToStringPtr(item.GetEndTime()),
		Elapsed:                 DurationPtrToStringPtr(item.GetElapsed()),
		ApplicationStatus:       item.GetState().String(),
		BlockchainStatus:        item.GetBlockchainState().String(),
		Dest:                    item.GetDest(),
		Miners:                  p.allocator.GetMinersFulfillingContract(item.GetID()),
	}
}

// TimePtrToStringPtr converts nullable time to nullable string
func TimePtrToStringPtr(t *time.Time) *string {
	if t != nil {
		a := t.Format(time.RFC3339)
		return &a
	}
	return nil
}

func DurationPtrToStringPtr(t *time.Duration) *string {
	if t != nil {
		a := formatDuration(*t)
		return &a
	}
	return nil
}

func mapHRToInt(m *allocator.Scheduler) map[string]int {
	hrFloat := m.GetHashrate().GetHashrateAvgGHSAll()
	hrInt := make(map[string]int, len(hrFloat))
	for k, v := range hrFloat {
		hrInt[k] = int(v)
	}
	return hrInt
}

func formatDuration(dur time.Duration) string {
	return dur.Round(time.Second).String()
}

func roundResourceEstimates(estimates map[string]float64) map[string]int {
	res := make(map[string]int, len(estimates))
	for k, v := range estimates {
		res[k] = int(v)
	}
	return res
}
