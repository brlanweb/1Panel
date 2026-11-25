package service

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
)

// In-memory runtime state of the compute scheduler. Realtime samples are
// never written to the database: they are recomputed after a restart
// within one collection round.

type computeHostState struct {
	rt       dto.ComputeHostRealtime
	prevFast *computeFastRaw
	prevAt   time.Time
	slow     *computeSlowRaw
	slowAt   time.Time
	fastAt   time.Time
	busy     bool
	fails    int
}

type computeInstState struct {
	m        dto.ComputeEngineMetrics
	prev     *computeEngineSample
	prevAt   time.Time
	inflight atomic.Int64
	// Router circuit breaker: consecutive upstream failures and the time
	// until which the instance is ejected.
	failStreak atomic.Int32
	ejectUntil atomic.Int64
	busy       bool
	// idleSince is the first sample with nothing running / waiting.
	idleSince time.Time
	// pressure counts consecutive scheduler rounds above the thresholds.
	pressure int
}

type computeStateStore struct {
	mu    sync.RWMutex
	hosts map[uint]*computeHostState
	insts map[uint]*computeInstState
	// watchUntil > now switches the collector to the fast cadence; the
	// page refreshes it with a heartbeat while it is visible.
	watchUntil atomic.Int64
}

var computeState = &computeStateStore{
	hosts: map[uint]*computeHostState{},
	insts: map[uint]*computeInstState{},
}

func (s *computeStateStore) host(id uint) *computeHostState {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.hosts[id]
	if h == nil {
		h = &computeHostState{}
		s.hosts[id] = h
	}
	return h
}

func (s *computeStateStore) inst(id uint) *computeInstState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.insts[id]
	if st == nil {
		st = &computeInstState{}
		s.insts[id] = st
	}
	return st
}

func (s *computeStateStore) hostRealtime(id uint) dto.ComputeHostRealtime {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if h := s.hosts[id]; h != nil {
		return h.rt
	}
	return dto.ComputeHostRealtime{}
}

func (s *computeStateStore) hostSlow(id uint) *computeSlowRaw {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if h := s.hosts[id]; h != nil {
		return h.slow
	}
	return nil
}

func (s *computeStateStore) instMetrics(id uint) dto.ComputeEngineMetrics {
	s.mu.RLock()
	st := s.insts[id]
	s.mu.RUnlock()
	if st == nil {
		return dto.ComputeEngineMetrics{}
	}
	s.mu.RLock()
	m := st.m
	s.mu.RUnlock()
	m.Inflight = st.inflight.Load()
	return m
}

func (s *computeStateStore) dropHost(id uint) {
	s.mu.Lock()
	delete(s.hosts, id)
	s.mu.Unlock()
}

func (s *computeStateStore) dropInst(id uint) {
	s.mu.Lock()
	delete(s.insts, id)
	s.mu.Unlock()
}

func (s *computeStateStore) watching() bool {
	return time.Now().Unix() < s.watchUntil.Load()
}
