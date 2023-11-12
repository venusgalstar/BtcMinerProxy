package contract

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources"
	hashrateContract "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/allocator"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/hashrate"
)

type ContractWatcherBuyer struct {
	// config
	contractCycleDuration  time.Duration
	validationStartTimeout time.Duration // time when validation kicks in
	shareTimeout           time.Duration // time to wait for the share to arrive, otherwise close contract
	hrErrorThreshold       float64       // hashrate relative error threshold for the contract to be considered fulfilling accurately

	terms                       *hashrateContract.EncryptedTerms
	state                       resources.ContractState
	validationStage             hashrateContract.ValidationStage
	fulfillmentStartedAt        *time.Time
	lastAcceptableHashrateCheck *time.Time
	hashrateErrorInterval       time.Duration

	tsk    *lib.Task
	cancel context.CancelFunc
	err    error
	doneCh chan struct{}

	//deps
	allocator      *allocator.Allocator
	globalHashrate *hashrate.GlobalHashrate
	log            interfaces.ILogger
}

func NewContractWatcherBuyer(
	terms *hashrateContract.EncryptedTerms,
	hashrateFactory func() *hashrate.Hashrate,
	allocator *allocator.Allocator,
	globalHashrate *hashrate.GlobalHashrate,
	log interfaces.ILogger,

	cycleDuration time.Duration,
	validationStartTimeout time.Duration,
	shareTimeout time.Duration,
	hrErrorThreshold float64,
	hashrateErrorInterval time.Duration,

) *ContractWatcherBuyer {
	return &ContractWatcherBuyer{
		terms:                  terms,
		state:                  resources.ContractStatePending,
		allocator:              allocator,
		globalHashrate:         globalHashrate,
		log:                    log,
		contractCycleDuration:  cycleDuration,
		validationStartTimeout: validationStartTimeout,
		shareTimeout:           shareTimeout,
		hrErrorThreshold:       hrErrorThreshold,
		validationStage:        hashrateContract.ValidationStageNotValidating,
		hashrateErrorInterval:  hashrateErrorInterval,
	}
}

func (p *ContractWatcherBuyer) StartFulfilling(ctx context.Context) {
	if p.state == resources.ContractStateRunning {
		p.log.Infof("buyer contract already fulfilling")
		return
	}
	p.log.Infof("buyer contract started fulfilling")
	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.doneCh = make(chan struct{})

	go func() {
		p.state = resources.ContractStateRunning
		p.err = p.Run(ctx)
		close(p.doneCh)
		p.state = resources.ContractStatePending
	}()
}

func (p *ContractWatcherBuyer) StopFulfilling() {
	p.cancel()
	<-p.doneCh
	p.log.Infof("buyer contract stopped fulfilling")
}

func (p *ContractWatcherBuyer) Done() <-chan struct{} {
	return p.doneCh
}

func (p *ContractWatcherBuyer) Err() error {
	if errors.Is(p.err, context.Canceled) {
		return ErrContractClosed
	}
	return p.err
}

func (p *ContractWatcherBuyer) SetData(data *hashrateContract.EncryptedTerms) {
	p.terms = data
}

func (p *ContractWatcherBuyer) Run(ctx context.Context) error {
	p.state = resources.ContractStateRunning
	startedAt := time.Now()
	p.fulfillmentStartedAt = &startedAt

	// instead of resetting write a method that creates separate counters for each worker at given moment of time
	p.globalHashrate.Reset(p.terms.ContractID)

	ticker := time.NewTicker(p.contractCycleDuration)
	defer ticker.Stop()

	endTime := p.GetEndTime()
	if endTime == nil {
		// if contract endtime is nil, it means it is ended
		return nil
	}

	endTimer := time.NewTimer(time.Until(*endTime))

	for {
		err := p.checkIncomingHashrate(ctx)
		if err != nil {
			return err
		}

		endTime := p.GetEndTime()
		if endTime == nil {
			// if contract endtime is nil, it means it is ended
			return nil
		}

		endTimer.Reset(time.Until(*endTime))

		select {
		case <-ctx.Done():
			if !endTimer.Stop() {
				<-endTimer.C
			}
			return ctx.Err()
		case <-endTimer.C:
			return nil
		case <-ticker.C:
			if !endTimer.Stop() {
				<-endTimer.C
			}
		}
	}
}

func (p *ContractWatcherBuyer) proceedToNextStage() {
	if p.validationStage == hashrateContract.ValidationStageNotValidating && p.isValidationStartTimeout() {
		p.validationStage = hashrateContract.ValidationStageValidating
		p.log.Infof("new validation stage %s", p.validationStage.String())
		return
	}

	if p.isContractExpired() {
		p.validationStage = hashrateContract.ValidationStageFinished
		p.log.Infof("new validation stage %s", p.validationStage.String())
		return
	}
}

func (p *ContractWatcherBuyer) checkIncomingHashrate(ctx context.Context) error {
	p.proceedToNextStage()

	isHashrateOK := p.isReceivingAcceptableHashrate()

	switch p.validationStage {
	case hashrateContract.ValidationStageNotValidating:
		lastShareTime, ok := p.globalHashrate.GetLastSubmitTime(p.getWorkerName())
		if !ok {
			lastShareTime = *p.fulfillmentStartedAt
		}
		if time.Since(lastShareTime) > p.shareTimeout {
			return fmt.Errorf("no share submitted within shareTimeout (%s)", p.shareTimeout)
		}
		return nil
	case hashrateContract.ValidationStageValidating:
		_, ok := p.globalHashrate.GetLastSubmitTime(p.getWorkerName())
		if !ok {
			errMsg := "on ValidationStateValidating there should be at least one share"
			p.log.DPanic(errMsg)
			return fmt.Errorf(errMsg)
		}
		// we don't need to continue to validate the share time as long as hashrate is consistent over time.
		// the seller algorithm is directly dependent on having time to make up for gaps in hashrate fullfillment over the course
		// of 1 hour.

		// if time.Since(lastShareTime) > p.shareTimeout {
		// 	return fmt.Errorf("no share submitted within shareTimeout (%s)", p.shareTimeout)
		// }
		if !isHashrateOK {
			return fmt.Errorf("contract is not delivering accurate hashrate")
		}
		return nil
	case hashrateContract.ValidationStageFinished:
		return fmt.Errorf("contract is finished")
	default:
		return fmt.Errorf("unknown validation state")
	}
}

func (p *ContractWatcherBuyer) isReceivingAcceptableHashrate() bool {
	// ignoring ok cause actualHashrate will be zero then
	actualHashrate, _ := p.globalHashrate.GetHashRateGHS(p.getWorkerName(), "mean")
	targetHashrateGHS := p.GetHashrateGHS()

	hrError := lib.RelativeError(targetHashrateGHS, actualHashrate)
	lastAcceptableHashrateCheck := time.Now()
	p.lastAcceptableHashrateCheck = &lastAcceptableHashrateCheck

	hrMsg := fmt.Sprintf(
		"elapsed %s worker %s, target GHS %.0f, actual GHS %.0f, error %.0f%%, threshold(%.0f%%)",
		p.GetElapsed().Round(time.Second), p.getWorkerName(), targetHashrateGHS, actualHashrate, hrError*100, p.hrErrorThreshold*100,
	)

	if hrError < p.hrErrorThreshold {
		p.log.Infof("contract is delivering accurately: %s", hrMsg)
		return true
	}

	if actualHashrate > targetHashrateGHS {
		p.log.Infof("contract is overdelivering: %s", hrMsg)
		// contract overdelivery is fine for buyer
		return true
	}

	p.log.Warnf("contract is underdelivering: %s", hrMsg)
	if p.lastAcceptableHashrateCheck != nil && time.Since(*p.lastAcceptableHashrateCheck) > p.hashrateErrorInterval {
		// only check hashrate accuracy every hashrateErrorInterval
		p.lastAcceptableHashrateCheck = nil

		p.log.Warnf("contract is underdelivering longer than: %v", hrMsg, p.hashrateErrorInterval)
		return false
	}

	return true
}

func (p *ContractWatcherBuyer) GetRole() resources.ContractRole {
	return resources.ContractRoleBuyer
}

func (p *ContractWatcherBuyer) GetDest() string {
	return ""
}

func (p *ContractWatcherBuyer) GetID() string {
	return p.terms.ContractID
}

func (p *ContractWatcherBuyer) GetHashrateGHS() float64 {
	return p.terms.Hashrate
}

func (p *ContractWatcherBuyer) GetSeller() string {
	return p.terms.Seller
}

func (p *ContractWatcherBuyer) GetBuyer() string {
	return p.terms.Buyer
}

func (p *ContractWatcherBuyer) GetStartedAt() *time.Time {
	return p.terms.StartsAt
}

func (p *ContractWatcherBuyer) GetElapsed() *time.Duration {
	if p.terms.StartsAt == nil {
		return nil
	}
	res := time.Since(*p.terms.StartsAt)
	return &res
}

func (p *ContractWatcherBuyer) GetDuration() time.Duration {
	return p.terms.Duration
}

func (p *ContractWatcherBuyer) GetEndTime() *time.Time {
	if p.terms.StartsAt == nil {
		return nil
	}
	endTime := p.terms.StartsAt.Add(p.terms.Duration)
	return &endTime
}

func (p *ContractWatcherBuyer) GetFulfillmentStartedAt() *time.Time {
	return p.fulfillmentStartedAt
}

func (p *ContractWatcherBuyer) GetState() resources.ContractState {
	return p.state
}

func (p *ContractWatcherBuyer) GetBlockchainState() hashrateContract.BlockchainState {
	return p.terms.State
}

func (p *ContractWatcherBuyer) GetValidationStage() hashrateContract.ValidationStage {
	return p.validationStage
}

func (p *ContractWatcherBuyer) GetResourceEstimates() map[string]float64 {
	return map[string]float64{
		ResourceEstimateHashrateGHS: p.GetHashrateGHS(),
	}
}

func (p *ContractWatcherBuyer) GetResourceEstimatesActual() map[string]float64 {
	res, _ := p.globalHashrate.GetHashRateGHSAll(p.getWorkerName())
	return res
}

func (p *ContractWatcherBuyer) GetResourceType() string {
	return ResourceTypeHashrate
}

func (p *ContractWatcherBuyer) isValidationStartTimeout() bool {
	return time.Since(*p.fulfillmentStartedAt) > p.validationStartTimeout
}

func (p *ContractWatcherBuyer) isContractExpired() bool {
	endTime := p.GetEndTime()
	if endTime == nil {
		return false
	}
	return time.Now().After(*endTime)
}

func (p *ContractWatcherBuyer) getWorkerName() string {
	return p.terms.ContractID
}
