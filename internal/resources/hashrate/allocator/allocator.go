package allocator

import (
	"context"
	"net/url"
	"sync"
	"time"

	gi "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/hashrate"
	"golang.org/x/exp/slices"
)

const (
	HashratePredictionAdjustment = 1
)

type MinerItem struct {
	ID    string
	HrGHS float64
}

type MinerItemJobScheduled struct {
	ID       string
	Job      float64
	Fraction float64
}

type AllocatorInterface interface {
	Run(ctx context.Context) error
	UpsertAllocation(ID string, hashrate float64, dest string, counter func(diff float64)) error
}

type Allocator struct {
	proxies    *lib.Collection[*Scheduler]
	proxyState sync.Map // map[string]bool map[proxyID]contractID
	log        gi.ILogger
}

func NewAllocator(proxies *lib.Collection[*Scheduler], log gi.ILogger) *Allocator {
	return &Allocator{
		proxies:    proxies,
		proxyState: sync.Map{},
		log:        log,
	}
}

func (p *Allocator) Run(ctx context.Context) error {
	return nil
}

func (p *Allocator) AllocateFullMinersForHR(ID string, hrGHS float64, dest *url.URL, duration time.Duration, onSubmit func(diff float64, ID string)) (minerIDs []string, deltaGHS float64) {
	miners := p.GetFreeMiners()

	for _, miner := range miners {
		minerGHS := miner.HrGHS
		if minerGHS <= hrGHS && minerGHS > 0 {
			minerIDs = append(minerIDs, miner.ID)
			proxy, ok := p.proxies.Load(miner.ID)
			if ok {
				proxy.AddTask(ID, dest, hashrate.GHSToJobSubmitted(minerGHS)*duration.Seconds(), onSubmit)
				p.log.Infof("miner %s allocated for %f GHS", miner.ID, minerGHS)
			}
			hrGHS -= minerGHS
		}
	}

	return minerIDs, hrGHS

	// TODO: improve miner selection
	// sort.Slice(miners, func(i, j int) bool {
	// 	return miners[i].HrGHS > miners[j].HrGHS
	// })
}

func (p *Allocator) AllocatePartialForHR(ID string, hrGHS float64, dest *url.URL, cycleDuration time.Duration, onSubmit func(diff float64, ID string)) (string, bool) {
	partialMiners := p.GetPartialMiners(cycleDuration)
	jobForCycle := hashrate.GHSToJobSubmitted(hrGHS) * cycleDuration.Seconds()

	// search in partially allocated miners
	for _, miner := range partialMiners {
		remainingJob := miner.Job / miner.Fraction
		if remainingJob >= jobForCycle {
			m, ok := p.proxies.Load(miner.ID)
			if ok {
				m.AddTask(ID, dest, jobForCycle, onSubmit)
				return miner.ID, true
			}
		}
	}

	// search in free miners
	freeMiners := p.GetFreeMiners()
	for _, miner := range freeMiners {
		remainingJob := hashrate.GHSToJobSubmitted(miner.HrGHS) * cycleDuration.Seconds()
		if remainingJob >= jobForCycle {
			m, ok := p.proxies.Load(miner.ID)
			if ok {
				m.AddTask(ID, dest, jobForCycle, onSubmit)
				return miner.ID, true
			}
		}
	}

	return "", false
}

func (p *Allocator) GetFreeMiners() []MinerItem {
	freeMiners := []MinerItem{}
	p.proxies.Range(func(item *Scheduler) bool {
		if item.IsVetting() {
			return true
		}
		if item.IsFree() {
			freeMiners = append(freeMiners, MinerItem{
				ID:    item.GetID(),
				HrGHS: item.HashrateGHS() * HashratePredictionAdjustment,
			})
		}
		return true
	})

	slices.SortStableFunc(freeMiners, func(i, j MinerItem) bool {
		return i.HrGHS > j.HrGHS
	})

	return freeMiners
}

func (p *Allocator) GetPartialMiners(contractCycleDuration time.Duration) []MinerItemJobScheduled {
	partialMiners := []MinerItemJobScheduled{}
	p.proxies.Range(func(item *Scheduler) bool {
		if item.IsVetting() {
			return true
		}
		if item.IsAcceptingTasks(contractCycleDuration) {
			job := item.GetTotalTaskJob() * HashratePredictionAdjustment
			fraction := hashrate.JobSubmittedToGHS(job) / (item.HashrateGHS() * HashratePredictionAdjustment)

			partialMiners = append(partialMiners, MinerItemJobScheduled{
				ID:       item.GetID(),
				Job:      job,
				Fraction: fraction,
			})
		}
		return true
	})

	slices.SortStableFunc(partialMiners, func(i, j MinerItemJobScheduled) bool {
		return i.Fraction > j.Fraction
	})

	return partialMiners
}

func (p *Allocator) GetMiners() *lib.Collection[*Scheduler] {
	return p.proxies
}

func (p *Allocator) GetMinersFulfillingContract(contractID string) []*DestItem {
	dests := []*DestItem{}
	p.GetMiners().Range(func(item *Scheduler) bool {
		if item.IsVetting() {
			return true
		}
		tasks := item.GetTasksByID(contractID)
		for _, task := range tasks {
			dests = append(dests, &DestItem{
				Dest: task.Dest.String(),
				Job:  task.RemainingJobToSubmit,
			})
		}
		return true
	})
	return dests
}

// CancelTasks cancels all tasks for a specified contractID
func (p *Allocator) CancelTasks(contractID string) {
	p.GetMiners().Range(func(item *Scheduler) bool {
		item.RemoveTasksByID(contractID)
		return true
	})
}
