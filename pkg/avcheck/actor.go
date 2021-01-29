package avcheck

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"
)

// each app config of the actor's bom must contain switchKey in its values section
// the value for this key must be bool and gets switched during every run of the actor
const switchKey = "switchThis"

type Actor struct {
	K8sClient      client.Client
	Log            logr.Logger
	InitialBom     *hubv1.ClusterBom
	ChangeInterval time.Duration
}

func (a *Actor) Start() {
	bomKey := types.NamespacedName{
		Namespace: a.InitialBom.GetNamespace(),
		Name:      a.InitialBom.GetName(),
	}

	a.Log = a.Log.WithValues(
		util.LogKeyClusterBomName, bomKey,
	)

	// in some cases the actor will already run while the manager is still initializing
	// this behavior produces an error in initializeBom()
	// it is therefore explicitly enforced that initializeBom() executes successfully
	initSuccessful := false
	for !initSuccessful {
		err := a.initializeBom()
		if err != nil {
			a.Log.Error(err, "cannot initialize bom")
			time.Sleep(5 * time.Second)
		} else {
			initSuccessful = true
		}
	}

	for {
		time.Sleep(a.ChangeInterval)
		err := a.modifyBom()
		if err != nil {
			a.Log.Error(err, "cannot modify bom")
		}
	}
}

func (a *Actor) initializeBom() error {
	ctx := context.Background()

	a.Log.V(util.LogLevelDebug).Info("initializing bom")

	bomKey := types.NamespacedName{
		Namespace: a.InitialBom.GetNamespace(),
		Name:      a.InitialBom.GetName(),
	}

	var bom hubv1.ClusterBom
	err := a.K8sClient.Get(ctx, bomKey, &bom)
	if err != nil {
		if apierrors.IsNotFound(err) {
			a.Log.Info("cannot find bom --> create bom")
			err = a.K8sClient.Create(ctx, a.InitialBom)
			if err != nil {
				return errors.Wrap(err, "cannot create bom")
			}
			a.Log.Info("bom successfully created")
			return nil
		}
		return errors.Wrap(err, "cannot get bom")
	}

	a.Log.V(util.LogLevelDebug).Info("found bom")
	if reflect.DeepEqual(bom.Spec, a.InitialBom.Spec) {
		a.Log.Info("bom already in initial state")
		return nil
	}

	a.Log.V(util.LogLevelDebug).Info("resetting bom to initial state")
	bom.Spec = a.InitialBom.Spec
	err = a.K8sClient.Update(ctx, &bom)
	if err != nil {
		return errors.Wrap(err, "cannot reset bom to initial state")
	}
	a.Log.V(util.LogLevelDebug).Info("bom successfully resetted to initial state")
	return nil
}

func (a *Actor) modifyBom() error {
	a.Log.V(util.LogLevelDebug).Info("modifying bom")

	ctx := context.Background()
	bomKey := types.NamespacedName{
		Namespace: a.InitialBom.GetNamespace(),
		Name:      a.InitialBom.GetName(),
	}

	var bom hubv1.ClusterBom
	err := a.K8sClient.Get(ctx, bomKey, &bom)
	if err != nil {
		return errors.Wrap(err, "cannot get bom")
	}

	for index := range bom.Spec.ApplicationConfigs {
		applConfig := bom.Spec.ApplicationConfigs[index]
		var values map[string]interface{}
		unmarshalErr := json.Unmarshal(bom.Spec.ApplicationConfigs[0].Values.Raw, &values)
		if unmarshalErr != nil {
			return errors.Wrapf(unmarshalErr, "cannot unmarshall values. applicationConfig = %+v", applConfig)
		}

		switchValStr, ok := values[switchKey]
		if !ok {
			return errors.Errorf("values[%s] is not set. applicationConfig = %+v", switchKey, applConfig)
		}

		switchVal, ok := switchValStr.(bool)
		if !ok {
			return errors.Errorf("values[%s] is not of type bool. applicationConfig = %+v", switchKey, applConfig)
		}

		values[switchKey] = !switchVal

		rawValues, marshalErr := json.Marshal(values)
		if marshalErr != nil {
			return errors.Wrapf(marshalErr, "cannot marshall values. applicationConfig = %+v", applConfig)
		}

		bom.Spec.ApplicationConfigs[index].Values = &runtime.RawExtension{Raw: rawValues}
	}

	err = a.K8sClient.Update(ctx, &bom)
	if err != nil {
		if apierrors.IsConflict(err) {
			a.Log.Info("cannot update bom due to conflict")
			return nil
		}
		return errors.Wrap(err, "cannot update bom")
	}

	a.Log.V(util.LogLevelDebug).Info("bom successfully modified")
	return nil
}
