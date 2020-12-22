package avcheck

import (
	"sync"
	"time"
)

type AVCheck struct {
	lastReconcileTime time.Time
	mux               sync.Mutex
}

func NewAVCheck() *AVCheck {
	obj := &AVCheck{
		lastReconcileTime: time.Now(),
		mux:               sync.Mutex{},
	}
	return obj
}

func (a *AVCheck) ReconcileCalled() {
	a.mux.Lock()
	a.lastReconcileTime = time.Now()
	a.mux.Unlock()
}

func (a *AVCheck) GetLastReconcileTime() time.Time {
	a.mux.Lock()
	lastReconcileTime := a.lastReconcileTime
	a.mux.Unlock()
	return lastReconcileTime
}
