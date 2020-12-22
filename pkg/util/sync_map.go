package util

import (
	"sync"
	"time"

	"github.com/go-logr/logr"
)

const logDuration = 1 * time.Minute

type ThreadCounterMap struct {
	syncMap          map[string]int
	allThreadCounter int
	mutex            sync.RWMutex
	logger           logr.Logger
	lastTime         time.Time
}

func NewThreadCounterMap(l logr.Logger) *ThreadCounterMap {
	return &ThreadCounterMap{
		syncMap: make(map[string]int),
		logger:  l,
	}
}

func (r *ThreadCounterMap) IncreaseEntryAndLog(s string) {
	logResult, allThreads := r.increaseEntry(s)

	if logResult != nil {
		r.logger.V(LogLevelWarning).Info("All threads", "threadcount", allThreads)

		for key, value := range logResult {
			r.logger.V(LogLevelWarning).Info("Threads for namespace", "namespace", key, "threadcount", value)
		}
	}
}

func (r *ThreadCounterMap) increaseEntry(s string) (map[string]int, int) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.allThreadCounter++

	i, ok := r.syncMap[s]

	if !ok {
		r.syncMap[s] = 1
	} else {
		r.syncMap[s] = i + 1
	}

	return r.getEntriesAfterTimeout()
}

func (r *ThreadCounterMap) ReduceEntry(s string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.allThreadCounter--

	i, ok := r.syncMap[s]

	if !ok {
		r.logger.Error(nil, "Reduce thread entry for missing key: "+s)
	} else {
		if i == 1 {
			delete(r.syncMap, s)
		} else {
			r.syncMap[s] = i - 1
		}
	}
}

func (r *ThreadCounterMap) getEntriesAfterTimeout() (map[string]int, int) {
	currentTime := time.Now()

	nextTime := r.lastTime.Add(logDuration)

	if currentTime.After(nextTime) {
		r.lastTime = currentTime

		newMap := make(map[string]int)

		for key, value := range r.syncMap {
			newMap[key] = value
		}

		return newMap, r.allThreadCounter
	}

	return nil, 0
}
