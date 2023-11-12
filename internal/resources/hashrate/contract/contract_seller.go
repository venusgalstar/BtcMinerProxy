package contract

import (
	"context"
	"errors"
	"net/url"
	"sync/atomic"
	"time"

	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate"
	hashrateContract "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/allocator"
	hr "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/hashrate"
	"golang.org/x/exp/slices"
)

type ContractWatcherSeller struct {
	terms *hashrateContract.Terms

	state                 resources.ContractState
	fullMiners            []string
	actualHRGHS           *hr.Hashrate
	fulfillmentStartedAt  *time.Time
	contractCycleDuration time.Duration

	tsk *lib.Task

	//deps
	allocator *allocator.Allocator
	log       interfaces.ILogger
}

func NewContractWatcherSeller(data *hashrateContract.Terms, cycleDuration time.Duration, hashrateFactory func() *hr.Hashrate, allocator *allocator.Allocator, log interfaces.ILogger) *ContractWatcherSeller {
	p := &ContractWatcherSeller{
		terms:                 data,
		state:                 resources.ContractStatePending,
		allocator:             allocator,
		fullMiners:            []string{},
		contractCycleDuration: cycleDuration,
		actualHRGHS:           hashrateFactory(),
		log:                   log,
	}
	p.tsk = lib.NewTaskFunc(func(ctx context.Context) error {
		p.state = resources.ContractStateRunning
		err := p.Run(ctx)
		p.state = resources.ContractStatePending
		return err
	})
	return p
}

func (p *ContractWatcherSeller) StartFulfilling(ctx context.Context) {
	if p.state == resources.ContractStateRunning {
		p.log.Warnf("contract already started fulfilling")
		return
	}
	p.log.Infof("contract started fulfilling")
	startedAt := time.Now()
	p.fulfillmentStartedAt = &startedAt
	p.tsk.Start(ctx)
}

func (p *ContractWatcherSeller) StopFulfilling() {
	<-p.tsk.Stop()
	p.allocator.CancelTasks(p.GetID())
	p.log.Infof("contract stopped fulfilling")
}

func (p *ContractWatcherSeller) Done() <-chan struct{} {
	return p.tsk.Done()
}

func (p *ContractWatcherSeller) Reset() {
	p.tsk = lib.NewTaskFunc(p.Run)
}

func (p *ContractWatcherSeller) Err() error {
	if errors.Is(p.tsk.Err(), context.Canceled) {
		return ErrContractClosed
	}
	return p.tsk.Err()
}

func (p *ContractWatcherSeller) SetData(data *hashrateContract.Terms) {
	p.terms = data
}

// Run is the main loop of the contract. It is responsible for allocating miners for the contract.
// Returns nil if the contract ended successfully, ErrClosed if the contract was closed before it ended.
func (p *ContractWatcherSeller) Run(ctx context.Context) error {
	partialDeliveryTargetGHS := p.GetHashrateGHS()
	thisCycleJobSubmitted := atomic.Uint64{}
	globalUnderdeliveryGHS := 0.0 // global contract underdelivery
	jobSubmittedFullMiners := atomic.Uint64{}
	jobSubmittedPartialMiners := atomic.Uint64{}

	for {
		jobSubmittedFullMiners.Store(0)
		jobSubmittedPartialMiners.Store(0)

		p.log.Debugf("new contract cycle:  partialDeliveryTargetGHS=%.0f elapsed %s",
			partialDeliveryTargetGHS, p.GetElapsed(),
		)
		if partialDeliveryTargetGHS > 0 {
			fullMiners, newRemainderGHS := p.allocator.AllocateFullMinersForHR(
				p.GetID(),
				partialDeliveryTargetGHS,
				p.getAdjustedDest(),
				p.GetDuration(),
				func(diff float64, ID string) {
					jobSubmittedFullMiners.Add(uint64(diff))
					p.actualHRGHS.OnSubmit(diff)
					thisCycleJobSubmitted.Add(uint64(diff))
				},
			)
			if len(fullMiners) > 0 {
				partialDeliveryTargetGHS = newRemainderGHS
				p.log.Infof("fully allocated %d miners, new partialDeliveryTargetGHS = %.0f", len(fullMiners), partialDeliveryTargetGHS)
				p.fullMiners = append(p.fullMiners, fullMiners...)
			} else {
				p.log.Debugf("no full miners were allocated for this contract")
			}

			minerID, ok := p.allocator.AllocatePartialForHR(
				p.GetID(),
				partialDeliveryTargetGHS,
				p.getAdjustedDest(),
				p.contractCycleDuration,
				func(diff float64, ID string) {
					jobSubmittedPartialMiners.Add(uint64(diff))
					p.actualHRGHS.OnSubmit(diff)
					thisCycleJobSubmitted.Add(uint64(diff))
				},
			)
			if ok {
				p.log.Debugf("remainderGHS: %.0f, was allocated by partial miners %v", partialDeliveryTargetGHS, minerID)
			} else {
				p.log.Warnf("remainderGHS: %.0f, was not allocated by partial miners", partialDeliveryTargetGHS)
			}
		}

		// in case of too much hashrate
		if partialDeliveryTargetGHS < 0 {
			p.log.Info("removing least powerful miner from contract")
			items := p.getAllocatedMinersSorted()

			if len(items) > 0 {
				minerToRemove := items[0].ID
				miner, ok := p.allocator.GetMiners().Load(minerToRemove)
				if ok {
					miner.RemoveTasksByID(p.GetID())
					p.log.Debugf("miner %s tasks removed", miner.GetID())
					newFullMiners := make([]string, len(p.fullMiners)-1)
					i := 0
					for _, minerID := range p.fullMiners {
						if minerID == minerToRemove {
							continue
						}
						newFullMiners[i] = minerID
						i++
					}
					p.fullMiners = newFullMiners

					// sets new target and restarts the cycle
					partialDeliveryTargetGHS = miner.HashrateGHS() + partialDeliveryTargetGHS
					continue
				}
			} else {
				p.log.Warnf("no miners found to be removed")
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Until(*p.GetEndTime())):
			p.log.Debugf("contract finished - now unix time: %v; local now unix time: %v", time.Now().Unix(), time.Now().Local().Unix())

			p.log.Debugf("contract finished - contract start time: %v", p.terms.StartsAt.Unix())
			p.log.Debugf("contract finished - contract duration: %v", p.terms.Duration.Seconds())
			p.log.Debugf("contract finished - contract end time: %v", p.GetEndTime().Unix())

			expectedJob := hr.GHSToJobSubmitted(p.GetHashrateGHS()) * p.GetDuration().Seconds()
			actualJob := p.actualHRGHS.GetTotalWork()
			undeliveredJob := expectedJob - actualJob
			undeliveredFraction := undeliveredJob / expectedJob

			for _, minerID := range p.fullMiners {
				miner, ok := p.allocator.GetMiners().Load(minerID)
				if !ok {
					continue
				}
				miner.RemoveTasksByID(p.GetID())
				p.log.Debugf("miner %s tasks removed", miner.GetID())
			}
			p.fullMiners = p.fullMiners[:0]

			// partial miners tasks are not reset because they are not allocated
			// for the full duration of the contract

			p.log.Infof("contract ended, undelivered work %d, undelivered fraction %.2f",
				int(undeliveredJob), undeliveredFraction)
			return nil
		case <-time.After(p.contractCycleDuration):
		}

		thisCycleActualGHS := p.jobToGHS(thisCycleJobSubmitted.Load())
		thisCycleUnderDeliveryGHS := p.GetHashrateGHS() - thisCycleActualGHS
		globalUnderdeliveryGHS += thisCycleUnderDeliveryGHS

		// plan for the next cycle is to compensate for the under delivery of the contract
		// partialDeliveryTargetGHS = partialDeliveryTargetGHS + globalUnderdeliveryGHS
		partialDeliveryTargetGHS = p.GetHashrateGHS() - p.GetFullMinersHR() + globalUnderdeliveryGHS

		thisCycleJobSubmitted.Store(0)

		p.log.Info("contract cycle ended",
			" thisCycleActualGHS=", int(thisCycleActualGHS),
			" thisCycleUnderDeliveryGHS=", int(thisCycleUnderDeliveryGHS),
			" globalUnderdeliveryGHS=", int(globalUnderdeliveryGHS),
			" partialDeliveryTargetGHS=", int(partialDeliveryTargetGHS),
			" jobSubmittedFullMiners=", int(p.jobToGHS(jobSubmittedFullMiners.Load())),
			" jobSubmittedPartialMiners=", int(p.jobToGHS(jobSubmittedPartialMiners.Load())),
		)
	}
}

func (p *ContractWatcherSeller) jobToGHS(value uint64) float64 {
	return hr.JobSubmittedToGHS(float64(value) / p.contractCycleDuration.Seconds())
}

func (p *ContractWatcherSeller) getAllocatedMinersSorted() []*allocator.MinerItem {
	var items []*allocator.MinerItem

	for _, minerID := range p.fullMiners {
		miner, ok := p.allocator.GetMiners().Load(minerID)
		if !ok {
			continue
		}
		items = append(items, &allocator.MinerItem{
			ID:    miner.GetID(),
			HrGHS: miner.HashrateGHS(),
		})
	}

	slices.SortStableFunc(items, func(a, b *allocator.MinerItem) bool {
		return b.HrGHS > a.HrGHS
	})

	return items
}

func (p *ContractWatcherSeller) GetFullMinersHR() float64 {
	var total float64
	for _, minerID := range p.fullMiners {
		miner, ok := p.allocator.GetMiners().Load(minerID)
		if !ok {
			continue
		}
		total += miner.HashrateGHS()
	}
	return total
}

// getAdjustedDest returns the destination url with the username set to the contractID
// this is required for the buyer to distinguish incoming hashrate between different contracts
func (p *ContractWatcherSeller) getAdjustedDest() *url.URL {
	if p.terms.Dest == nil {
		return nil
	}
	dest := *p.terms.Dest
	lib.SetUserName(&dest, p.terms.ContractID)
	return &dest
}

// ShouldBeRunning checks blockchain state and expiration time and returns true if the contract should be running
func (p *ContractWatcherSeller) ShouldBeRunning() bool {
	endTime := p.GetEndTime()
	if endTime == nil {
		return false
	}
	return p.GetBlockchainState() == hashrate.BlockchainStateRunning && p.GetEndTime().After(time.Now())
}

//
// Public getters
//

func (p *ContractWatcherSeller) GetRole() resources.ContractRole {
	return resources.ContractRoleSeller
}

func (p *ContractWatcherSeller) GetDest() string {
	if dest := p.getAdjustedDest(); dest != nil {
		return dest.String()
	}
	return ""
}

func (p *ContractWatcherSeller) GetDuration() time.Duration {
	return p.terms.Duration
}

func (p *ContractWatcherSeller) GetStartedAt() *time.Time {
	return p.terms.StartsAt
}

func (p *ContractWatcherSeller) GetEndTime() *time.Time {
	if p.terms.StartsAt == nil {
		return nil
	}
	endTime := p.terms.StartsAt.Add(p.terms.Duration)
	return &endTime
}

func (p *ContractWatcherSeller) GetFulfillmentStartedAt() *time.Time {
	return p.fulfillmentStartedAt
}

func (p *ContractWatcherSeller) GetElapsed() *time.Duration {
	if p.terms.StartsAt == nil {
		return nil
	}
	res := time.Since(*p.terms.StartsAt)
	return &res
}

func (p *ContractWatcherSeller) GetID() string {
	return p.terms.ContractID
}

func (p *ContractWatcherSeller) GetHashrateGHS() float64 {
	return p.terms.Hashrate
}

func (p *ContractWatcherSeller) GetSeller() string {
	return p.terms.Seller
}

func (p *ContractWatcherSeller) GetBuyer() string {
	return p.terms.Buyer
}

func (p *ContractWatcherSeller) GetState() resources.ContractState {
	return p.state
}

func (p *ContractWatcherSeller) GetBlockchainState() hashrate.BlockchainState {
	return p.terms.State
}

func (p *ContractWatcherSeller) GetResourceType() string {
	return ResourceTypeHashrate
}

func (p *ContractWatcherSeller) GetResourceEstimates() map[string]float64 {
	return map[string]float64{
		ResourceEstimateHashrateGHS: p.GetHashrateGHS(),
	}
}

func (p *ContractWatcherSeller) GetResourceEstimatesActual() map[string]float64 {
	return p.actualHRGHS.GetHashrateAvgGHSAll()
}

func (p *ContractWatcherSeller) GetValidationStage() hashrateContract.ValidationStage {
	return hashrateContract.ValidationStageNotApplicable // only for buyer
}
