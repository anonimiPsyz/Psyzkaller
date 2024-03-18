// Copyright 2024 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package corpus

import (
	"context"
	"sync"

	"github.com/google/syzkaller/pkg/cover"
	"github.com/google/syzkaller/pkg/hash"
	"github.com/google/syzkaller/pkg/rpctype"
	"github.com/google/syzkaller/pkg/signal"
	"github.com/google/syzkaller/prog"
)

// Corpus object represents a set of syzkaller-found programs that
// cover the kernel up to the currently reached frontiers.
type Corpus struct {
	ctx     context.Context
	mu      sync.RWMutex
	progs   map[string]*Item
	signal  signal.Signal // signal of inputs in corpus
	updates chan<- NewItemEvent
	ProgramsList
}

func NewCorpus(ctx context.Context) *Corpus {
	return NewMonitoredCorpus(ctx, nil)
}

func NewMonitoredCorpus(ctx context.Context, updates chan<- NewItemEvent) *Corpus {
	return &Corpus{
		ctx:     ctx,
		progs:   make(map[string]*Item),
		updates: updates,
	}
}

// It may happen that a single program is relevant because of several
// sysalls. In that case, there will be several ItemUpdate entities.
type ItemUpdate struct {
	Call     int
	RawCover []uint32
}

// Item objects are to be treated as immutable, otherwise it's just
// too hard to synchonize accesses to them across the whole project.
// When Corpus updates one of its items, it saves a copy of it.
type Item struct {
	Sig      string
	Call     int
	Prog     *prog.Prog
	ProgData []byte // to save some Serialize() calls
	HasAny   bool   // whether the prog contains squashed arguments
	Signal   signal.Signal
	Cover    []uint32
	Updates  []ItemUpdate
}

func (item Item) StringCall() string {
	return stringCall(item.Prog, item.Call)
}

// RPCInputShort() does not include coverage.
func (item Item) RPCInputShort() rpctype.Input {
	return rpctype.Input{
		Call:   item.Call,
		Prog:   item.ProgData,
		Signal: item.Signal.Serialize(),
	}
}

func stringCall(p *prog.Prog, call int) string {
	if call != -1 {
		return p.Calls[call].Meta.Name
	}
	return ".extra"
}

type NewInput struct {
	Prog     *prog.Prog
	Call     int
	Signal   signal.Signal
	Cover    []uint32
	RawCover []uint32
}

func (item NewInput) StringCall() string {
	return stringCall(item.Prog, item.Call)
}

func (item NewInput) RPCInput() rpctype.Input {
	return rpctype.Input{
		Call:     item.Call,
		Prog:     item.Prog.Serialize(),
		Signal:   item.Signal.Serialize(),
		Cover:    item.Cover,
		RawCover: item.RawCover,
	}
}

type NewItemEvent struct {
	Sig      string
	Exists   bool
	ProgData []byte
}

func (corpus *Corpus) Save(inp NewInput) {
	progData := inp.Prog.Serialize()
	sig := hash.String(progData)

	corpus.mu.Lock()
	defer corpus.mu.Unlock()

	update := ItemUpdate{
		Call:     inp.Call,
		RawCover: inp.RawCover,
	}
	exists := false
	if old, ok := corpus.progs[sig]; ok {
		exists = true
		newSignal := old.Signal.Copy()
		newSignal.Merge(inp.Signal)
		var newCover cover.Cover
		newCover.Merge(old.Cover)
		newCover.Merge(inp.Cover)
		newItem := &Item{
			Sig:      sig,
			Prog:     old.Prog,
			ProgData: progData,
			Call:     old.Call,
			HasAny:   old.HasAny,
			Signal:   newSignal,
			Cover:    newCover.Serialize(),
			Updates:  append([]ItemUpdate{}, old.Updates...),
		}
		const maxUpdates = 32
		if len(newItem.Updates) < maxUpdates {
			newItem.Updates = append(newItem.Updates, update)
		}
		corpus.progs[sig] = newItem
	} else {
		corpus.progs[sig] = &Item{
			Sig:      sig,
			Call:     inp.Call,
			Prog:     inp.Prog,
			ProgData: progData,
			HasAny:   inp.Prog.ContainsAny(),
			Signal:   inp.Signal,
			Cover:    inp.Cover,
			Updates:  []ItemUpdate{update},
		}
		corpus.saveProgram(inp.Prog, inp.Signal)
	}
	corpus.signal.Merge(inp.Signal)
	if corpus.updates != nil {
		select {
		case <-corpus.ctx.Done():
		case corpus.updates <- NewItemEvent{
			Sig:      sig,
			Exists:   exists,
			ProgData: progData,
		}:
		}
	}
}

func (corpus *Corpus) DiffSignal(s signal.Signal) signal.Signal {
	corpus.mu.RLock()
	defer corpus.mu.RUnlock()
	return corpus.signal.Diff(s)
}

func (corpus *Corpus) Signal() signal.Signal {
	corpus.mu.RLock()
	defer corpus.mu.RUnlock()
	return corpus.signal.Copy()
}

func (corpus *Corpus) Items() []*Item {
	corpus.mu.RLock()
	defer corpus.mu.RUnlock()
	ret := make([]*Item, 0, len(corpus.progs))
	for _, item := range corpus.progs {
		ret = append(ret, item)
	}
	return ret
}

func (corpus *Corpus) Item(sig string) *Item {
	corpus.mu.RLock()
	defer corpus.mu.RUnlock()
	return corpus.progs[sig]
}

// Stat is a snapshot of the relevant current state figures.
type Stat struct {
	Progs  int
	Signal int
}

func (corpus *Corpus) Stat() Stat {
	corpus.mu.RLock()
	defer corpus.mu.RUnlock()
	return Stat{
		Progs:  len(corpus.progs),
		Signal: len(corpus.signal),
	}
}

type CallCov struct {
	Count int
	Cover cover.Cover
}

func (corpus *Corpus) CallCover() map[string]*CallCov {
	corpus.mu.RLock()
	defer corpus.mu.RUnlock()
	calls := make(map[string]*CallCov)
	for _, inp := range corpus.progs {
		call := inp.StringCall()
		if calls[call] == nil {
			calls[call] = new(CallCov)
		}
		cc := calls[call]
		cc.Count++
		cc.Cover.Merge(inp.Cover)
	}
	return calls
}
