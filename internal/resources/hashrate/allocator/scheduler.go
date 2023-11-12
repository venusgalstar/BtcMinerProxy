package allocator

import (
	"context"
	"net/url"
	"sync"
	"time"

	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	h "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/hashrate"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/proxy"
)

type Task struct {
	ID                   string
	Dest                 *url.URL
	RemainingJobToSubmit float64
	OnSubmit             func(diff float64, ID string)
	cancelCh             chan struct{}
}

// Scheduler is a proxy wrapper that can schedule one-time tasks to different destinations
type Scheduler struct {
	// config
	minerVettingDuration time.Duration
	hashrateCounterID    string
	primaryDest          *url.URL

	// state
	totalTaskJob     float64
	newTaskSignal    chan struct{}
	resetTasksSignal chan struct{}
	tasks            lib.Stack[Task]

	// deps
	proxy StratumProxyInterface
	log   interfaces.ILogger
}

func NewScheduler(proxy StratumProxyInterface, hashrateCounterID string, defaultDest *url.URL, minerVettingDuration time.Duration, log interfaces.ILogger) *Scheduler {
	return &Scheduler{
		primaryDest:          defaultDest,
		hashrateCounterID:    hashrateCounterID,
		minerVettingDuration: minerVettingDuration,
		newTaskSignal:        make(chan struct{}, 1),
		proxy:                proxy,
		log:                  log,
	}
}

func (p *Scheduler) GetID() string {
	return p.proxy.GetID()
}

func (p *Scheduler) Run(ctx context.Context) error {
	err := p.proxy.Connect(ctx)
	if err != nil {
		return err // handshake error
	}

	proxyTask := lib.NewTaskFunc(p.proxy.Run)
	proxyTask.Start(ctx)

	select {
	case <-ctx.Done():
		<-proxyTask.Done()
		return ctx.Err()
	case <-proxyTask.Done():
		return proxyTask.Err()
	default:
	}

	p.primaryDest = p.proxy.GetDest()

	for {
		// do tasks
		for {
			p.resetTasksSignal = make(chan struct{})
			task, ok := p.tasks.Peek()
			if !ok {
				break
			}
			select {
			case <-task.cancelCh:
				p.log.Debugf("task cancelled %s", task.ID)
				p.tasks.Pop()
				continue
			default:
			}

			jobDone := make(chan struct{})
			jobDoneOnce := sync.Once{}
			p.log.Debugf("start doing task for job ID %s, for job %.0f", task.ID, task.RemainingJobToSubmit)
			p.totalTaskJob -= task.RemainingJobToSubmit
			err := p.proxy.SetDest(ctx, task.Dest, func(diff float64) {
				task.RemainingJobToSubmit -= diff
				task.OnSubmit(diff, p.proxy.GetID())
				if task.RemainingJobToSubmit <= 0 {
					jobDoneOnce.Do(func() {
						p.log.Debugf("finished doing task for job %s", task.ID)
						close(jobDone)
					})
				}
			})
			if err != nil {
				return err
			}

			select {
			case <-ctx.Done():
				<-proxyTask.Done()
				return ctx.Err()
			case <-proxyTask.Done():
				return proxyTask.Err()
			case <-p.resetTasksSignal:
				close(jobDone)
				p.log.Debugf("tasks resetted")
			case <-task.cancelCh:
				close(jobDone)
				p.log.Debugf("task cancelled %s", task.ID)
			case <-jobDone:
			}

			p.tasks.Pop()
		}

		select {
		case <-ctx.Done():
			<-proxyTask.Done()
			return ctx.Err()
		case <-proxyTask.Done():
			return proxyTask.Err()
		case <-p.newTaskSignal:
			continue
		default:
		}

		// remaining time serve default destination
		err := p.proxy.SetDest(ctx, p.primaryDest, nil)
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			<-proxyTask.Done()
			return ctx.Err()
		case <-proxyTask.Done():
			return proxyTask.Err()
		case <-p.newTaskSignal:
		}
	}
}

func (p *Scheduler) AddTask(ID string, dest *url.URL, jobSubmitted float64, onSubmit func(diff float64, ID string)) {
	shouldSignal := p.tasks.Size() == 0
	p.tasks.Push(Task{
		ID:                   ID,
		Dest:                 dest,
		RemainingJobToSubmit: jobSubmitted,
		OnSubmit:             onSubmit,
		cancelCh:             make(chan struct{}),
	})
	p.totalTaskJob += jobSubmitted
	if shouldSignal {
		p.newTaskSignal <- struct{}{}
	}
	p.log.Debugf("added new task, dest: %s, for jobSubmitted: %.0f, totalTaskJob: %.0f", dest, jobSubmitted, p.totalTaskJob)
}

// TODO: ensure it is concurrency safe
func (p *Scheduler) RemoveTasksByID(ID string) {
	p.tasks.Range(func(task Task) bool {
		if task.ID == ID {
			close(task.cancelCh)
		}
		return true
	})
}

func (p *Scheduler) ResetTasks() {
	p.tasks.Clear()
	close(p.resetTasksSignal)
}

func (p *Scheduler) GetTaskCount() int {
	return p.tasks.Size()
}

func (p *Scheduler) GetTasksByID(ID string) []Task {
	var tasks []Task
	for _, tsk := range p.tasks {
		if tsk.ID == ID {
			tasks = append(tasks, tsk)
		}
	}
	return tasks
}

func (p *Scheduler) GetTotalTaskJob() float64 {
	return p.totalTaskJob
}

func (p *Scheduler) IsFree() bool {
	return p.tasks.Size() == 0
}

// AcceptsTasks returns true if there are vacant space for tasks for provided interval
func (p *Scheduler) IsAcceptingTasks(duration time.Duration) bool {
	totalJob := 0.0
	for _, tsk := range p.tasks {
		totalJob += tsk.RemainingJobToSubmit
	}
	maxJob := h.GHSToJobSubmitted(p.HashrateGHS()) * duration.Seconds()
	return p.tasks.Size() > 0 && totalJob < maxJob
}

func (p *Scheduler) SetPrimaryDest(dest *url.URL) {
	p.primaryDest = dest
	p.newTaskSignal <- struct{}{}
}

func (p *Scheduler) HashrateGHS() float64 {
	hr, ok := p.proxy.GetHashrate().GetHashrateAvgGHSCustom(p.hashrateCounterID)
	if !ok {
		panic("hashrate counter not found")
	}
	return hr
}

func (p *Scheduler) GetStatus() MinerStatus {
	if p.IsVetting() {
		return MinerStatusVetting
	}

	if p.IsFree() {
		return MinerStatusFree
	}

	return MinerStatusBusy
}

func (p *Scheduler) GetCurrentDifficulty() float64 {
	return p.proxy.GetDifficulty()
}

func (p *Scheduler) GetCurrentDest() *url.URL {
	return p.proxy.GetDest()
}

func (p *Scheduler) GetWorkerName() string {
	return p.proxy.GetSourceWorkerName()
}

func (p *Scheduler) GetConnectedAt() time.Time {
	return p.proxy.GetMinerConnectedAt()
}

func (p *Scheduler) GetStats() interface{} {
	return p.proxy.GetStats()
}

func (p *Scheduler) IsVetting() bool {
	return p.GetUptime() < p.minerVettingDuration
}

func (p *Scheduler) GetUptime() time.Duration {
	return time.Since(p.proxy.GetMinerConnectedAt())
}

func (p *Scheduler) GetDestConns() *map[string]string {
	return p.proxy.GetDestConns()
}

func (p *Scheduler) GetHashrate() proxy.Hashrate {
	return p.proxy.GetHashrate()
}

func (p *Scheduler) GetDestinations() []*DestItem {
	dests := make([]*DestItem, p.tasks.Size())

	for i, t := range p.tasks {
		dests[i] = &DestItem{
			Dest: t.Dest.String(),
			Job:  t.RemainingJobToSubmit,
		}
	}

	return dests
}

type DestItem struct {
	Dest string
	Job  float64
}
