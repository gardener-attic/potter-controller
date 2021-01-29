package controllersdi

import (
	"context"
	"strings"
	"time"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/synchronize"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	lastUpdateStartedKey  = "lastUpdateStarted"
	lastUpdateFinishedKey = "lastUpdateFinished"
	blockIDKey            = "blockID"
	blockUntilKey         = "blockUntil"
)

type ReconcileClock interface {
	Now() time.Time
	Sleep(duration time.Duration)
}

type RealReconcileClock struct{}

func (r *RealReconcileClock) Now() time.Time {
	return time.Now()
}

func (r *RealReconcileClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

type ReconcileController struct {
	Client              client.Client
	UncachedClient      synchronize.UncachedClient
	HubControllerClient synchronize.UncachedClient
	Log                 logr.Logger
	Scheme              *runtime.Scheme
	Clock               ReconcileClock
	ConfigMapKey        types.NamespacedName
	UniqueID            string
	SyncDisabled        bool
}

func (r *ReconcileController) Reconcile(reconcileInterval, restartKappInterval time.Duration) {
	log := r.Log.WithValues(
		util.LogKeyInterval, reconcileInterval.String(),
		util.LogKeyConfigmap, r.ConfigMapKey,
		util.LogKeyCorrelationID, uuid.New().String(),
	)

	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, log)

	// After a restart, we do not immediately start the reconciliation loop (usually). The start time of the last loop
	// is stored in a configmap so that we can compute when the next loop should start. We sleep until then.
	// Only if the last start date is not available, or the next start is overdue, we start directly.

	for {
		lastStartTime, err := r.readLastStartTime(ctx)
		if err != nil {
			r.Clock.Sleep(10 * time.Second)
		} else {
			// Compute start time for the next reconcile and decide whether to sleep or to work
			nextStartTime := lastStartTime.Add(reconcileInterval)
			now := r.Clock.Now()
			if nextStartTime.After(now) {
				// Sleep
				sleepDuration := nextStartTime.Sub(now)
				log.V(util.LogLevelDebug).Info("sleeping", "until", nextStartTime)
				r.Clock.Sleep(sleepDuration)
			} else {
				// Work
				ok := r.reconcileAll(ctx, restartKappInterval)

				if !ok {
					// The reconcile either failed or was blocked by a parallel controller instance
					r.Clock.Sleep(5 * time.Minute)
				}
			}
		}
	}
}

func (r *ReconcileController) reconcileAll(ctx context.Context, restartKappInterval time.Duration) bool {
	log := util.GetLoggerFromContext(ctx)

	ok, err := r.block(ctx, 5*time.Minute)
	if !ok || err != nil {
		return false
	}
	defer r.unblock(ctx)

	log.V(util.LogLevelDebug).Info("Processing reconcile loop")

	startTime := r.Clock.Now()
	r.handleAllClusterBoms(ctx)
	r.cleanup(ctx, restartKappInterval)
	finishTime := r.Clock.Now()

	err = r.upsertConfigMap(ctx, startTime, finishTime)
	if err != nil {
		log.Error(err, "error updating reconcile map")
		return false
	}

	return true
}

func (r *ReconcileController) readLastStartTime(ctx context.Context) (time.Time, error) {
	log := util.GetLoggerFromContext(ctx)
	log.V(util.LogLevelDebug).Info("reading last start time from reconcile map")

	var configMap corev1.ConfigMap

	err := r.HubControllerClient.GetUncached(ctx, r.ConfigMapKey, &configMap)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.V(util.LogLevelDebug).Info("creating reconcile map")

			err = r.upsertConfigMap(ctx, time.Time{}, time.Time{})
			if err != nil {
				log.Error(err, "error creating reconcile map")
				return time.Time{}, err
			}

			return time.Time{}, nil
		}

		log.Error(err, "error fetching reconcile map")
		return time.Time{}, err
	}

	data := configMap.Data
	lastStartTimeText := data[lastUpdateStartedKey]
	lastStartTime, err := unmarshalTime([]byte(lastStartTimeText))
	if err != nil {
		log.Error(err, "error unmarshaling start time", "startTime", lastStartTimeText)
		return time.Time{}, err
	}

	return lastStartTime, nil
}

func (r *ReconcileController) upsertConfigMap(ctx context.Context, startTime, finishTime time.Time) error { // nolint
	log := util.GetLoggerFromContext(ctx).WithValues("startTime", startTime, "finishTime", finishTime)

	startTimeText, err := marshalTime(startTime)
	if err != nil {
		log.Error(err, "error marshaling start time", "startTime", startTime)
		return err
	}
	lastUpdateStarted := string(startTimeText)

	finishTimeText, err := marshalTime(finishTime)
	if err != nil {
		log.Error(err, "error marshaling finish time", "finishTime", finishTime)
		return err
	}
	lastUpdateFinished := string(finishTimeText)

	var configMap corev1.ConfigMap

	err = r.HubControllerClient.GetUncached(ctx, r.ConfigMapKey, &configMap)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.V(util.LogLevelDebug).Info("creating configmap")

			data := make(map[string]string)
			data[lastUpdateStartedKey] = lastUpdateStarted
			data[lastUpdateFinishedKey] = lastUpdateFinished

			configMap = corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: r.ConfigMapKey.Namespace,
					Name:      r.ConfigMapKey.Name,
				},
				Data: data,
			}

			err = r.HubControllerClient.Create(ctx, &configMap)
			if err != nil {
				log.Error(err, "error creating configmap")
				return err
			}

			return nil
		}

		log.Error(err, "error reading configmap")
		return err
	}

	configMap.Data[lastUpdateStartedKey] = lastUpdateStarted
	configMap.Data[lastUpdateFinishedKey] = lastUpdateFinished

	log.V(util.LogLevelDebug).Info("updating configmap")
	err = r.HubControllerClient.Update(ctx, &configMap)
	if err != nil {
		log.Error(err, "error updating configmap")
		return err
	}

	return nil
}

func (r *ReconcileController) handleAllClusterBoms(ctx context.Context) {
	log := util.GetLoggerFromContext(ctx)
	log.V(util.LogLevelDebug).Info("start reconcile loop")

	var clusterBomList hubv1.ClusterBomList

	err := r.Client.List(ctx, &clusterBomList)
	if err != nil {
		log.Error(err, "error listing clusterboms")
	} else {
		for i := range clusterBomList.Items {
			clusterBom := &clusterBomList.Items[i]
			clusterBomKey := util.GetKey(clusterBom)

			r.autoDeleteClusterlessBom(ctx, clusterBomKey)

			r.markForReconcile(ctx, clusterBom)

			time.Sleep(10 * time.Second)

			// reblock
			ok, err := r.block(ctx, 5*time.Minute)
			if !ok || err != nil {
				log.Error(err, "interrupting reconcile loop")
				return
			}
		}
	}
}

// Deletes the clusterbom if all of the following holds:
// 1. auto-deletion is configured, i.e. spec/autoDelete/clusterBomAge is specified and has a positive value,
// 2. the clusterbom is older than the duration spec/autoDelete/clusterBomAge (in minutes), and
// 3. the target cluster does not exist (more precisely, the secret with its kubeconfig)
func (r *ReconcileController) autoDeleteClusterlessBom(ctx context.Context, clusterBomKey *types.NamespacedName) {
	log := util.GetLoggerFromContext(ctx).WithValues(util.LogKeyClusterBomName, clusterBomKey)

	var clusterBom hubv1.ClusterBom
	err := r.Client.Get(ctx, *clusterBomKey, &clusterBom)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "error fetching clusterbom to check auto-delete")
		}
		return
	}

	if clusterBom.Spec.AutoDelete != nil && clusterBom.Spec.AutoDelete.ClusterBomAge > 0 {
		creationTime := clusterBom.ObjectMeta.CreationTimestamp
		now := r.Clock.Now()
		if creationTime.Add(time.Duration(clusterBom.Spec.AutoDelete.ClusterBomAge) * time.Minute).Before(now) {
			secretKey := util.GetSecretKeyFromClusterBom(&clusterBom)
			log = log.WithValues(
				"secretKey", secretKey,
				"clusterBomAge", clusterBom.Spec.AutoDelete.ClusterBomAge,
				"creationTime", creationTime)

			var secret corev1.Secret
			err = r.Client.Get(ctx, *secretKey, &secret)
			if err != nil {
				if apierrors.IsNotFound(err) {
					log.V(util.LogLevelWarning).Info("auto-deleting clusterbom")
					err = r.Client.Delete(ctx, &clusterBom)
					if err != nil {
						log.Error(err, "error auto-deleting clusterbom")
						return
					}
				} else {
					log.Error(err, "error fetching secret to check auto-delete")
					return
				}
			}
		}
	}
}

func (r *ReconcileController) markForReconcile(ctx context.Context, clusterBom *hubv1.ClusterBom) {
	log := util.GetLoggerFromContext(ctx).WithValues(util.LogKeyClusterBomName, util.GetKey(clusterBom))

	util.AddAnnotation(clusterBom, util.AnnotationKeyReconcile, util.AnnotationValueReconcile)

	err := r.Client.Update(ctx, clusterBom)
	if err != nil {
		log.Error(err, "error updating clusterbom with reconcile annotation")
		return
	}
}

func (r *ReconcileController) block(ctx context.Context, duration time.Duration) (bool, error) {
	log := util.GetLoggerFromContext(ctx)

	var configMap corev1.ConfigMap

	err := r.HubControllerClient.GetUncached(ctx, r.ConfigMapKey, &configMap)
	if err != nil {
		log.Error(err, "error fetching reconcile map for block")
		return false, err
	}

	takeOverAllowed := false
	blockID, ok := configMap.Data[blockIDKey]
	if !ok || blockID == r.UniqueID {
		takeOverAllowed = true
	} else {
		blockUntilString := configMap.Data[blockUntilKey]
		blockUntil, err2 := unmarshalTime([]byte(blockUntilString))
		if err2 != nil {
			log.Error(err2, "error unmarshaling block time", "time", blockUntilString)
			return false, err2
		}

		if blockUntil.Add(time.Minute).Before(time.Now()) {
			log.Error(nil, "found expired reconcile block")
			takeOverAllowed = true
		}
	}

	if !takeOverAllowed {
		return false, nil
	}

	blockTime := time.Now().Add(duration)
	blockTimeBytes, err := marshalTime(blockTime)
	if err != nil {
		log.Error(err, "error marshaling block time", "blockTime", blockTime)
		return false, err
	}
	configMap.Data[blockUntilKey] = string(blockTimeBytes)
	configMap.Data[blockIDKey] = r.UniqueID

	err = r.HubControllerClient.Update(ctx, &configMap)
	if err != nil {
		if util.IsConcurrentModificationErr(err) {
			log.V(util.LogLevelDebug).Info("concurrent modification for updating reconcile map for block " + err.Error())
			return false, nil
		}
		log.Error(err, "error updating reconcile map for block")
		return false, err
	}

	return true, nil
}

func (r *ReconcileController) unblock(ctx context.Context) {
	log := util.GetLoggerFromContext(ctx)

	var configMap corev1.ConfigMap

	err := r.HubControllerClient.GetUncached(ctx, r.ConfigMapKey, &configMap)
	if err != nil {
		log.Error(err, "error fetching reconcile map for unblock")
		return
	}

	if r.UniqueID != configMap.Data[blockIDKey] {
		log.Error(err, "error unblocking reconcile map: wrong id")
		return
	}

	delete(configMap.Data, blockUntilKey)
	delete(configMap.Data, blockIDKey)

	err = r.HubControllerClient.Update(ctx, &configMap)
	if err != nil {
		log.Error(err, "error updating reconcile map for unblock")
	}
}

func (r *ReconcileController) cleanup(ctx context.Context, restartKappInterval time.Duration) {
	log := util.GetLoggerFromContext(ctx)

	var clusterBomList hubv1.ClusterBomList
	err := r.Client.List(ctx, &clusterBomList)
	if err != nil {
		log.Error(err, "error listing clusterbom objects when deleting sync objects")
		return
	}

	// prepare clusterboms
	clusterBomMap := make(map[types.NamespacedName]*hubv1.ClusterBom)

	for i := range clusterBomList.Items {
		clusterBom := &clusterBomList.Items[i]
		cluterbomKey := types.NamespacedName{
			Namespace: clusterBom.Namespace,
			Name:      clusterBom.Name,
		}
		clusterBomMap[cluterbomKey] = clusterBom
	}

	r.cleanupClusterBomSyncObject(ctx, clusterBomMap)

	r.cleanupSecrets(ctx)

	r.restartKappController(ctx, restartKappInterval)
}

func (r *ReconcileController) deleteSyncObject(ctx context.Context, key types.NamespacedName) {
	log := util.GetLoggerFromContext(ctx)

	blockObject := synchronize.NewBlockObject(nil, r.SyncDisabled)
	blockOk, _, err := blockObject.Block(ctx, &key, r.UncachedClient, 5*time.Minute, true)
	if err != nil {
		log.Error(err, "error get block for deleting sync object")
		return
	} else if !blockOk {
		log.V(util.LogLevelDebug).Info("not possible to get block for deleting sync object")
		return
	}

	defer blockObject.Release(ctx, &key, true)

	err = blockObject.DeleteBlock(ctx, &key, r.UncachedClient)

	if err != nil {
		log.Error(err, "error deleting sync object")
	}
}

func (r *ReconcileController) cleanupClusterBomSyncObject(ctx context.Context, clusterBomMap map[types.NamespacedName]*hubv1.ClusterBom) {
	log := util.GetLoggerFromContext(ctx)

	// cleanup sync objects
	var clusterBomSyncList hubv1.ClusterBomSyncList
	err := r.Client.List(ctx, &clusterBomSyncList)
	if err != nil {
		log.Error(err, "error listing clusterbom sync objects")
	} else {
		for i := range clusterBomSyncList.Items {
			clusterBomSync := &clusterBomSyncList.Items[i]

			key := types.NamespacedName{
				Namespace: clusterBomSync.Namespace,
				Name:      clusterBomSync.Name,
			}

			if _, ok := clusterBomMap[key]; !ok {
				now := time.Now()

				if clusterBomSync.Spec.Until.Time.Add(5 * time.Hour).Before(now) {
					r.deleteSyncObject(ctx, key)
				}
			}
		}
	}
}

func marshalTime(t time.Time) ([]byte, error) {
	wrappedTime := metav1.Time{Time: t}
	return wrappedTime.MarshalJSON()
}

func unmarshalTime(bytes []byte) (time.Time, error) {
	wrappedTime := metav1.Time{}
	err := (&wrappedTime).UnmarshalJSON(bytes)
	if err != nil {
		return time.Time{}, err
	}
	return wrappedTime.Time, nil
}

func (r *ReconcileController) cleanupSecrets(ctx context.Context) {
	log := util.GetLoggerFromContext(ctx)

	// read secrets for secret values
	secretList := corev1.SecretList{}
	err := r.Client.List(ctx, &secretList, client.MatchingLabels{hubv1.LabelPurpose: util.PurposeSecretValues})
	if err != nil {
		log.Error(err, "Error fetching secret list")
		return
	}

	for i := range secretList.Items {
		secret := &secretList.Items[i]

		if !secret.ObjectMeta.CreationTimestamp.Time.Add(time.Hour).Before(time.Now()) {
			continue
		}

		clusterBomName, ok := secret.GetLabels()[hubv1.LabelClusterBomName]
		if !ok {
			log.Error(nil, "Found secret with purpose secret-values, but without clusterbom label", util.LogKeySecretName, util.GetKey(secret))
			continue
		}

		clusterBomKey := types.NamespacedName{
			Namespace: secret.GetNamespace(),
			Name:      clusterBomName,
		}

		// read clusterbom
		clusterBom := hubv1.ClusterBom{}
		err = r.Client.Get(ctx, clusterBomKey, &clusterBom)
		if err != nil {
			if apierrors.IsNotFound(err) {
				err = deleteSecret(ctx, secret, r.Client, r.HubControllerClient)
				if err != nil {
					log.Error(err, "Error deleting secret", util.LogKeySecretName, util.GetKey(secret), util.LogKeyClusterBomName, clusterBomKey)
					continue
				}
			} else { // nolint
				log.Error(err, "Error fetching clusterbom", util.LogKeyClusterBomName, clusterBomKey)
				continue
			}
		}
	}
}

var lastUpdate = time.Now() // nolint

func (r *ReconcileController) restartKappController(ctx context.Context, restartKappInterval time.Duration) {
	if restartKappInterval < time.Minute || time.Since(lastUpdate) < restartKappInterval {
		return
	}

	lastUpdate = time.Now()

	log := util.GetLoggerFromContext(ctx)

	// cleanup sync objects
	var pods corev1.PodList
	err := r.HubControllerClient.ListUncached(ctx, &pods, client.InNamespace("hub-controller"))
	if err != nil {
		log.Error(err, "restartKappController: error listing pods")
		return
	}

	log.V(util.LogLevelWarning).Info("restartKappController: try to restart kapp controller")

	for i := range pods.Items {
		pod := &pods.Items[i]

		if strings.HasPrefix(pod.Name, "kapp-controller") {
			log.V(util.LogLevelWarning).Info("restartKappController: restart kapp controller: " + pod.Name)
			err = r.HubControllerClient.Delete(ctx, pod)

			if err != nil {
				log.Error(err, "restartKappController: error removing kapp controller")
			}
			return
		}
	}
}
