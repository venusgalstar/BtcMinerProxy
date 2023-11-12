package validator

import (
	"errors"
	"fmt"

	gi "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/interfaces"
	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/lib"
	sm "gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/proxy/stratumv1_message"
)

const (
	JOB_CACHE_SIZE = 30
)

var (
	ErrJobNotFound    = errors.New("job not found")
	ErrDuplicateShare = errors.New("duplicate share")
	ErrLowDifficulty  = errors.New("low difficulty")
)

type Validator struct {
	// state
	jobs               *lib.BoundStackMap[*MiningJob]
	versionRollingMask string

	// deps
	log gi.ILogger
}

func NewValidator(log gi.ILogger) *Validator {
	return &Validator{
		jobs:               lib.NewBoundStackMap[*MiningJob](JOB_CACHE_SIZE),
		versionRollingMask: "00000000",
		log:                log,
	}
}

func (v *Validator) SetVersionRollingMask(mask string) {
	v.versionRollingMask = mask
}

func (v *Validator) AddNewJob(msg *sm.MiningNotify, diff float64, xn1 string, xn2size int) {
	job := NewMiningJob(msg, diff, xn1, xn2size)
	v.jobs.Push(msg.GetJobID(), job)
}

func (v *Validator) HasJob(jobID string) bool {
	_, ok := v.jobs.Get(jobID)
	return ok
}

func (v *Validator) ValidateAndAddShare(msg *sm.MiningSubmit) (float64, error) {
	var (
		job *MiningJob
		ok  bool
	)

	if job, ok = v.jobs.Get(msg.GetJobId()); !ok {
		return 0, ErrJobNotFound
	}

	if job.CheckDuplicateAndAddShare(msg) {
		return 0, ErrDuplicateShare
	}

	diff, ok := ValidateDiff(job.extraNonce1, uint(job.extraNonce2Size), uint64(job.diff), v.versionRollingMask, job.notify, msg)
	diffFloat := float64(diff)
	if !ok {
		err := lib.WrapError(ErrLowDifficulty, fmt.Errorf("expected %.2f actual %d", job.diff, diff))
		v.log.Warnf(err.Error())
		v.log.Warnf("xn=%s, xnsize=%d, diff=%d, vrmsk=%s", job.extraNonce1, uint(job.extraNonce2Size), uint64(job.diff), v.versionRollingMask)
		return diffFloat, err
	}

	return diffFloat, nil
}

func (v *Validator) GetLatestJob() (*MiningJob, bool) {
	job, ok := v.jobs.At(-1)
	if !ok {
		return nil, false
	}
	return job.Copy(), true
}
