package hashrate

import (
	"time"

	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
)

type GlobalHashrate struct {
	data      *lib.Collection[*WorkerHashrateModel]
	hrFactory HashrateFactory
}

func NewGlobalHashrate(hrFactory HashrateFactory) *GlobalHashrate {
	return &GlobalHashrate{
		data:      lib.NewCollection[*WorkerHashrateModel](),
		hrFactory: hrFactory,
	}
}

func (t *GlobalHashrate) OnSubmit(workerName string, diff float64) {
	actual, _ := t.data.LoadOrStore(&WorkerHashrateModel{ID: workerName, hr: t.hrFactory()})
	actual.OnSubmit(diff)
}

func (t *GlobalHashrate) GetLastSubmitTime(workerName string) (tm time.Time, ok bool) {
	record, ok := t.data.Load(workerName)
	if !ok {
		return time.Time{}, false
	}
	return record.hr.GetLastSubmitTime(), true
}

func (t *GlobalHashrate) GetHashRateGHS(workerName string, counterID string) (hrGHS float64, ok bool) {
	record, ok := t.data.Load(workerName)
	if !ok {
		return 0, false
	}
	return record.GetHashRateGHS(counterID)
}

func (t *GlobalHashrate) GetHashRateGHSAll(workerName string) (hrGHS map[string]float64, ok bool) {
	record, ok := t.data.Load(workerName)
	if !ok {
		return nil, false
	}
	return record.GetHashrateAvgGHSAll(), true
}

func (t *GlobalHashrate) GetTotalWork(workerName string) (work float64, ok bool) {
	record, ok := t.data.Load(workerName)
	if !ok {
		return 0, false
	}
	return record.hr.GetTotalWork(), true
}

func (t *GlobalHashrate) GetAll() map[string]time.Time {
	data := make(map[string]time.Time)
	t.data.Range(func(item *WorkerHashrateModel) bool {
		data[item.ID] = item.hr.GetLastSubmitTime()
		return true
	})
	return data
}

func (t *GlobalHashrate) Range(f func(m *WorkerHashrateModel) bool) {
	t.data.Range(func(item *WorkerHashrateModel) bool {
		return f(item)
	})
}

func (t *GlobalHashrate) Reset(workerName string) {
	t.data.Delete(workerName)
}

type WorkerHashrateModel struct {
	ID string
	hr *Hashrate
}

func (m *WorkerHashrateModel) GetID() string {
	return m.ID
}

func (m *WorkerHashrateModel) OnSubmit(diff float64) {
	m.hr.OnSubmit(diff)
}

func (m *WorkerHashrateModel) GetHashRateGHS(counterID string) (float64, bool) {
	return m.hr.GetHashrateAvgGHSCustom(counterID)
}

func (m *WorkerHashrateModel) GetHashrateAvgGHSAll() map[string]float64 {
	return m.hr.GetHashrateAvgGHSAll()
}

func (m *WorkerHashrateModel) GetLastSubmitTime() time.Time {
	return m.hr.GetLastSubmitTime()
}
