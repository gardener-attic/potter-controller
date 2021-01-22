package synchronize

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-logr/logr"

	"github.com/gardener/potter-controller/pkg/util"

	hubv1 "github.com/gardener/potter-controller/api/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
)

// TODO
// - implement release method

const timeBuffer = time.Minute

type mutexMapEntry struct {
	mutex   sync.Mutex
	counter int
}

type mutexMap struct {
	globalMutex sync.Mutex
	syncMap     map[types.NamespacedName]*mutexMapEntry
}

func newMutexMap() *mutexMap {
	return &mutexMap{
		globalMutex: sync.Mutex{},
		syncMap:     make(map[types.NamespacedName]*mutexMapEntry),
	}
}

func (r *mutexMap) getMutex(key *types.NamespacedName) *sync.Mutex {
	r.globalMutex.Lock()
	defer r.globalMutex.Unlock()

	cm := r.syncMap[*key]

	if cm == nil {
		cm = &mutexMapEntry{}
		r.syncMap[*key] = cm
	}

	cm.counter++
	return &cm.mutex
}

func (r *mutexMap) releaseMutex(key *types.NamespacedName) {
	r.globalMutex.Lock()
	defer r.globalMutex.Unlock()

	cm := r.syncMap[*key]
	if cm != nil {
		cm.counter--

		if cm.counter == 0 {
			delete(r.syncMap, *key)
		}
	}
}

func NewBlockObject(excludedBoms []types.NamespacedName, syncDisabled bool) *BlockObject {
	return &BlockObject{
		mutexHelper:  newMutexMap(),
		uniqueID:     string(uuid.NewUUID()),
		excludedBoms: excludedBoms,
		clock:        &util.RealClock{},
		syncDisabled: syncDisabled,
	}
}

type BlockObject struct {
	mutexHelper  *mutexMap
	uniqueID     string
	excludedBoms []types.NamespacedName
	clock        util.Clock
	syncDisabled bool
}

// try to get block
// in case of an error returns (false, undefined, error)
// in case of successful getting the log return (true, undefined, nil)
// otherwise return (false, duration to wait until retry, nil)
func (r *BlockObject) Block(ctx context.Context, clusterbomKey *types.NamespacedName, clt UncachedClient,
	duration time.Duration, ignoreExclusionList bool) (bool, time.Duration, error) {
	if r.syncDisabled {
		return true, time.Second, nil
	}

	if !ignoreExclusionList && r.isExcluded(clusterbomKey) {
		return true, time.Second, nil
	}

	syncObject := r.mutexHelper.getMutex(clusterbomKey)
	defer r.mutexHelper.releaseMutex(clusterbomKey)

	syncObject.Lock()
	defer syncObject.Unlock()

	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	var clusterBomSync hubv1.ClusterBomSync

	err := clt.GetUncached(ctx, *clusterbomKey, &clusterBomSync)
	if err != nil {
		if apierrors.IsNotFound(err) {
			clusterBomSync = r.newClusterBomSync(clusterbomKey, r.uniqueID, duration)
			err = clt.Create(ctx, &clusterBomSync)

			if err != nil {
				tmpErr := clt.GetUncached(ctx, *clusterbomKey, &clusterBomSync)
				if tmpErr != nil {
					log.Error(err, "Error creating block")
					log.Error(tmpErr, "Error fetching block after creation")
					return false, 1 * time.Second, err
				}
			}

			log.V(util.LogLevelDebug).Info("Block created or fetched", "blocked-until", clusterBomSync.Spec.Until.Time,
				"blocked-by", clusterBomSync.Spec.ID, "own-id", r.uniqueID)
		} else {
			log.Error(err, "Error initial fetching block")
			return false, 1 * time.Second, err
		}
	}

	// Check whether the block belongs someone else and is not expired
	now := r.clock.Now()
	if clusterBomSync.Spec.ID != r.uniqueID && now.Before(clusterBomSync.Spec.Until.Time.Add(timeBuffer)) {
		log.V(util.LogLevelDebug).Info("Blocked by someone else", "now", now, "blocked-until", clusterBomSync.Spec.Until.Time,
			"blocked-by", clusterBomSync.Spec.ID, "own-id", r.uniqueID)

		retryDuration := clusterBomSync.Spec.Until.Time.Add(timeBuffer).Add(time.Second * 5).Sub(r.clock.Now())

		if retryDuration < 1*time.Second {
			retryDuration = 1 * time.Second
		}

		return false, retryDuration, nil
	}

	// We can take over the block, resp. extend it. Check whether an update is necessary to change the owner or expiration date.
	until := now.Add(duration)
	if clusterBomSync.Spec.Until.Time.Before(until) || clusterBomSync.Spec.ID != r.uniqueID {
		isOwnBlockAndNotExpired := clusterBomSync.Spec.ID == r.uniqueID && now.Before(clusterBomSync.Spec.Until.Time)

		clusterBomSync.Spec.ID = r.uniqueID
		clusterBomSync.Spec.Until = metav1.Time{Time: until}

		err := clt.Update(ctx, &clusterBomSync)

		if err != nil {
			if isOwnBlockAndNotExpired {
				log.Error(err, "Error updating block")
			} else {
				log.V(util.LogLevelWarning).Info("Problem updating block", "message", err.Error())
			}

			return false, time.Second * 10, nil
		}

		log.V(util.LogLevelDebug).Info("Block updated", "now", now, "blocked-until", clusterBomSync.Spec.Until.Time, "blocked-by", clusterBomSync.Spec.ID)
	}

	return true, time.Second * 1, nil
}

func (r *BlockObject) Reblock(ctx context.Context, clusterbomKey *types.NamespacedName, clt UncachedClient, duration time.Duration, ignoreExclusionList bool) (bool, error) {
	if r.syncDisabled {
		return true, nil
	}

	if !ignoreExclusionList && r.isExcluded(clusterbomKey) {
		return true, nil
	}

	syncObject := r.mutexHelper.getMutex(clusterbomKey)
	defer r.mutexHelper.releaseMutex(clusterbomKey)

	syncObject.Lock()
	defer syncObject.Unlock()

	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	var clusterBomSync hubv1.ClusterBomSync

	err := clt.GetUncached(ctx, *clusterbomKey, &clusterBomSync)
	if err != nil {
		log.Error(err, "Error fetching block for reblock")
		return false, err
	}

	// check clusterBomSync
	now := r.clock.Now()
	if clusterBomSync.Spec.ID != r.uniqueID {
		// blocked by someone else
		message := "Reblock failed; wrong id"
		err := errors.New(message)
		log.Error(err, message, "now", now, "blocked-until", clusterBomSync.Spec.Until.Time,
			"blocked-by", clusterBomSync.Spec.ID, "own-id", r.uniqueID)
		return false, err
	} else if now.Add(timeBuffer).After(clusterBomSync.Spec.Until.Time) {
		message := "Reblock failed; block is expired"
		err := errors.New(message)
		log.Error(err, message, "now", now, "blocked-until", clusterBomSync.Spec.Until.Time,
			"blocked-by", clusterBomSync.Spec.ID)
		return false, err
	}

	until := now.Add(duration)

	if clusterBomSync.Spec.Until.Time.Before(until) {
		clusterBomSync.Spec.Until = metav1.Time{Time: until}

		err := clt.Update(ctx, &clusterBomSync)

		if err != nil {
			log.Error(err, "Reblock failed; update failed")
			return false, err
		}

		log.V(util.LogLevelDebug).Info("Block updated; reblock", "blocked-until", clusterBomSync.Spec.Until.Time,
			"blocked-by", clusterBomSync.Spec.ID)
	}

	return true, nil
}

func (r *BlockObject) Release(ctx context.Context, clusterbomKey *types.NamespacedName, ignoreExclusionList bool) {
	if r.syncDisabled {
		return
	}

	if !ignoreExclusionList && r.isExcluded(clusterbomKey) {
		return
	}
}

func (r *BlockObject) DeleteBlock(ctx context.Context, clusterbomKey *types.NamespacedName, clt UncachedClient) error {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	var clusterBomSync hubv1.ClusterBomSync

	log.V(util.LogLevelDebug).Info("Deleting block")

	err := clt.GetUncached(ctx, *clusterbomKey, &clusterBomSync)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Error(err, "Block not found before deletion")
			return nil
		}

		log.Error(err, "Error fetching block before deletion")
		return err
	}

	log.V(util.LogLevelDebug).Info("Found block to be deleted")

	err = clt.Delete(ctx, &clusterBomSync)
	if err != nil {
		log.Error(err, "Error deleting block")
		return err
	}

	log.V(util.LogLevelDebug).Info("Block deleted")

	return nil
}

func (r *BlockObject) isExcluded(key *types.NamespacedName) bool {
	for _, nextBom := range r.excludedBoms {
		if nextBom == *key {
			return true
		}
	}

	return false
}

func (r *BlockObject) newClusterBomSync(key *types.NamespacedName, uniqueID string, duration time.Duration) hubv1.ClusterBomSync {
	currentTime := r.clock.Now()
	until := currentTime.Add(duration)

	return hubv1.ClusterBomSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
		Spec: hubv1.ClusterBomSyncSpec{
			ID:        uniqueID,
			Timestamp: metav1.Time{Time: currentTime},
			Until:     metav1.Time{Time: until},
		},
	}
}
