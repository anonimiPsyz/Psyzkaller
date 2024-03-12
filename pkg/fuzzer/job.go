// Copyright 2024 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package fuzzer

import (
	"fmt"
	"math/rand"

	"github.com/google/syzkaller/pkg/cover"
	"github.com/google/syzkaller/pkg/hash"
	"github.com/google/syzkaller/pkg/ipc"
	"github.com/google/syzkaller/pkg/rpctype"
	"github.com/google/syzkaller/pkg/signal"
	"github.com/google/syzkaller/prog"
)

const (
	smashPrio int64 = iota + 1
	genPrio
	triagePrio
	candidatePrio
	candidateTriagePrio
)

type job interface {
	run(fuzzer *Fuzzer)
	saveID(id int64)
	priority() priority
}

type ProgTypes int

const (
	ProgCandidate ProgTypes = 1 << iota
	ProgMinimized
	ProgSmashed
	ProgNormal ProgTypes = 0
)

type jobPriority struct {
	prio priority
}

func newJobPriority(base int64) jobPriority {
	prio := append(make(priority, 0, 2), base)
	return jobPriority{prio}
}

func (jp jobPriority) priority() priority {
	return jp.prio
}

// If we prioritize execution requests only by the base priorities of their origin
// jobs, we risk letting 1000s of simultaneous jobs slowly progress in parallel.
// It's better to let same-prio jobs that were started earlier finish first.
// saveID() allows Fuzzer to attach this sub-prio at the moment of job creation.
func (jp *jobPriority) saveID(id int64) {
	jp.prio = append(jp.prio, id)
}

func genProgRequest(fuzzer *Fuzzer, rnd *rand.Rand) *Request {
	p := fuzzer.target.Generate(rnd,
		prog.RecommendedCalls,
		fuzzer.ChoiceTable())
	return &Request{
		Prog:       p,
		NeedSignal: true,
		stat:       statGenerate,
	}
}

func mutateProgRequest(fuzzer *Fuzzer, rnd *rand.Rand) *Request {
	p := fuzzer.Corpus.chooseProgram(rnd)
	if p == nil {
		return nil
	}
	newP := p.Clone()
	newP.Mutate(rnd,
		prog.RecommendedCalls,
		fuzzer.ChoiceTable(),
		fuzzer.Config.NoMutateCalls,
		fuzzer.Corpus.Programs(),
	)
	return &Request{
		Prog:       newP,
		NeedSignal: true,
		stat:       statFuzz,
	}
}

func candidateRequest(input Candidate) *Request {
	flags := ProgCandidate
	if input.Minimized {
		flags |= ProgMinimized
	}
	if input.Smashed {
		flags |= ProgSmashed
	}
	return &Request{
		Prog:       input.Prog,
		NeedSignal: true,
		stat:       statCandidate,
		flags:      flags,
	}
}

// triageJob are programs for which we noticed potential new coverage during
// first execution. But we are not sure yet if the coverage is real or not.
// During triage we understand if these programs in fact give new coverage,
// and if yes, minimize them and add to corpus.
type triageJob struct {
	p         *prog.Prog
	call      int
	info      ipc.CallInfo
	newSignal signal.Signal
	flags     ProgTypes
	jobPriority
}

func triageJobPrio(flags ProgTypes) jobPriority {
	if flags&ProgCandidate > 0 {
		return newJobPriority(candidateTriagePrio)
	}
	return newJobPriority(triagePrio)
}

func (job *triageJob) run(fuzzer *Fuzzer) {
	callName := ".extra"
	logCallName := "extra"
	if job.call != -1 {
		callName = job.p.Calls[job.call].Meta.Name
		logCallName = fmt.Sprintf("call #%v %v", job.call, callName)
	}
	fuzzer.Logf(3, "triaging input for %v (new signal=%v)", logCallName, job.newSignal.Len())
	// Compute input coverage and non-flaky signal for minimization.
	info, stop := job.deflake(fuzzer)
	if stop || info.newStableSignal.Empty() {
		return
	}
	if job.flags&ProgMinimized == 0 {
		stop = job.minimize(fuzzer, info.newStableSignal)
		if stop {
			return
		}
	}
	data := job.p.Serialize()
	fuzzer.Logf(2, "added new input for %q to the corpus:\n%s",
		logCallName, string(data))
	if job.flags&ProgSmashed == 0 {
		fuzzer.startJob(&smashJob{
			p:           job.p.Clone(),
			call:        job.call,
			jobPriority: newJobPriority(smashPrio),
		})
	}
	fuzzer.Corpus.Save(job.p, info.stableSignal, hash.Hash(data))
	if fuzzer.Config.NewInputs != nil {
		select {
		case <-fuzzer.ctx.Done():
		case fuzzer.Config.NewInputs <- rpctype.Input{
			Call:     callName,
			CallID:   job.call,
			Prog:     data,
			Signal:   info.stableSignal.Serialize(),
			Cover:    info.cover.Serialize(),
			RawCover: info.rawCover,
		}:
		}
	}
}

type deflakedCover struct {
	stableSignal    signal.Signal
	newStableSignal signal.Signal
	cover           cover.Cover
	rawCover        []uint32
}

func (job *triageJob) deflake(fuzzer *Fuzzer) (info deflakedCover, stop bool) {
	const signalRuns = 3
	var notExecuted int
	for i := 0; i < signalRuns; i++ {
		result := fuzzer.exec(job, &Request{
			Prog:         job.p,
			NeedSignal:   true,
			NeedCover:    true,
			NeedRawCover: fuzzer.Config.FetchRawCover,
			stat:         statTriage,
		})
		if result.Stop {
			stop = true
			return
		}
		if !reexecutionSuccess(result.Info, &job.info, job.call) {
			// The call was not executed or failed.
			notExecuted++
			if notExecuted >= signalRuns/2+1 {
				stop = true
				return // if happens too often, give up
			}
			continue
		}
		thisSignal, thisCover := getSignalAndCover(job.p, result.Info, job.call)
		if len(info.rawCover) == 0 && fuzzer.Config.FetchRawCover {
			info.rawCover = thisCover
		}
		if i == 0 {
			info.stableSignal = thisSignal
			info.newStableSignal = job.newSignal.Intersection(thisSignal)
		} else {
			info.stableSignal = info.stableSignal.Intersection(thisSignal)
			info.newStableSignal = info.newStableSignal.Intersection(thisSignal)
		}
		if info.newStableSignal.Empty() {
			return
		}
		info.cover.Merge(thisCover)
	}
	return
}

func (job *triageJob) minimize(fuzzer *Fuzzer, newSignal signal.Signal) (stop bool) {
	const minimizeAttempts = 3
	job.p, job.call = prog.Minimize(job.p, job.call, false,
		func(p1 *prog.Prog, call1 int) bool {
			if stop {
				return false
			}
			for i := 0; i < minimizeAttempts; i++ {
				result := fuzzer.exec(job, &Request{
					Prog:       p1,
					NeedSignal: true,
					stat:       statMinimize,
				})
				if result.Stop {
					stop = true
					return false
				}
				info := result.Info
				if !reexecutionSuccess(info, &job.info, call1) {
					// The call was not executed or failed.
					continue
				}
				thisSignal, _ := getSignalAndCover(p1, info, call1)
				if newSignal.Intersection(thisSignal).Len() == newSignal.Len() {
					return true
				}
			}
			return false
		})
	return stop
}

func reexecutionSuccess(info *ipc.ProgInfo, oldInfo *ipc.CallInfo, call int) bool {
	if info == nil || len(info.Calls) == 0 {
		return false
	}
	if call != -1 {
		// Don't minimize calls from successful to unsuccessful.
		// Successful calls are much more valuable.
		if oldInfo.Errno == 0 && info.Calls[call].Errno != 0 {
			return false
		}
		return len(info.Calls[call].Signal) != 0
	}
	return len(info.Extra.Signal) != 0
}

func getSignalAndCover(p *prog.Prog, info *ipc.ProgInfo, call int) (signal.Signal, []uint32) {
	inf := &info.Extra
	if call != -1 {
		inf = &info.Calls[call]
	}
	return signal.FromRaw(inf.Signal, signalPrio(p, inf, call)), inf.Cover
}

type smashJob struct {
	p    *prog.Prog
	call int
	jobPriority
}

func (job *smashJob) run(fuzzer *Fuzzer) {
	fuzzer.Logf(2, "smashing the program %s (call=%d):", job.p, job.call)
	if fuzzer.Config.Comparisons && job.call >= 0 {
		fuzzer.startJob(&hintsJob{
			p:           job.p.Clone(),
			call:        job.call,
			jobPriority: newJobPriority(smashPrio),
		})
	}

	const iters = 100
	rnd := fuzzer.rand()
	for i := 0; i < iters; i++ {
		p := job.p.Clone()
		p.Mutate(rnd, prog.RecommendedCalls,
			fuzzer.ChoiceTable(),
			fuzzer.Config.NoMutateCalls,
			fuzzer.Corpus.Programs())
		result := fuzzer.exec(job, &Request{
			Prog:       p,
			NeedSignal: true,
			stat:       statSmash,
		})
		if result.Stop {
			return
		}
		if fuzzer.Config.Collide {
			result := fuzzer.exec(job, &Request{
				Prog: randomCollide(p, rnd),
				stat: statCollide,
			})
			if result.Stop {
				return
			}
		}
	}
	if fuzzer.Config.FaultInjection && job.call >= 0 {
		job.faultInjection(fuzzer)
	}
}

func randomCollide(origP *prog.Prog, rnd *rand.Rand) *prog.Prog {
	if rnd.Intn(5) == 0 {
		// Old-style collide with a 20% probability.
		p, err := prog.DoubleExecCollide(origP, rnd)
		if err == nil {
			return p
		}
	}
	if rnd.Intn(4) == 0 {
		// Duplicate random calls with a 20% probability (25% * 80%).
		p, err := prog.DupCallCollide(origP, rnd)
		if err == nil {
			return p
		}
	}
	p := prog.AssignRandomAsync(origP, rnd)
	if rnd.Intn(2) != 0 {
		prog.AssignRandomRerun(p, rnd)
	}
	return p
}

func (job *smashJob) faultInjection(fuzzer *Fuzzer) {
	for nth := 1; nth <= 100; nth++ {
		fuzzer.Logf(2, "injecting fault into call %v, step %v",
			job.call, nth)
		newProg := job.p.Clone()
		newProg.Calls[job.call].Props.FailNth = nth
		result := fuzzer.exec(job, &Request{
			Prog: job.p,
			stat: statSmash,
		})
		if result.Stop {
			return
		}
		info := result.Info
		if info != nil && len(info.Calls) > job.call &&
			info.Calls[job.call].Flags&ipc.CallFaultInjected == 0 {
			break
		}
	}
}

type hintsJob struct {
	p    *prog.Prog
	call int
	jobPriority
}

func (job *hintsJob) run(fuzzer *Fuzzer) {
	// First execute the original program to dump comparisons from KCOV.
	p := job.p
	result := fuzzer.exec(job, &Request{
		Prog:      p,
		NeedHints: true,
		stat:      statSeed,
	})
	if result.Stop || result.Info == nil {
		return
	}
	// Then mutate the initial program for every match between
	// a syscall argument and a comparison operand.
	// Execute each of such mutants to check if it gives new coverage.
	var stop bool
	p.MutateWithHints(job.call, result.Info.Calls[job.call].Comps,
		func(p *prog.Prog) {
			if stop {
				return
			}
			result := fuzzer.exec(job, &Request{
				Prog:       p,
				NeedSignal: true,
				stat:       statHint,
			})
			stop = stop || result.Stop
		})
}
