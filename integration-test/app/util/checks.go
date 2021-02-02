package util

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"time"

	hubv1 "github.com/gardener/potter-controller/api/v1"

	external "github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	"github.com/vmware-tanzu/carvel-kapp-controller/pkg/apis/kappctrl/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const numberOfRetries = 144

func CheckClusterReconcileAnnotation(ctx context.Context, clusterBom *hubv1.ClusterBom, cl *ClusterBomClient) {
	f := func(storedClusterBom *hubv1.ClusterBom, clusterBom *hubv1.ClusterBom) bool {
		annotations := storedClusterBom.GetAnnotations()
		_, ok := annotations[hubv1.AnnotationValueLandscaperManaged]

		if !ok {
			Write("Reconcile annotation removed")
			return true
		} else { // nolint
			Write("Reconcile annotation still exists")
			return false
		}
	}

	CheckClusterBomGeneric(ctx, clusterBom, cl, f)
}

func CheckClusterBomCondition(ctx context.Context, clusterBom *hubv1.ClusterBom, cl *ClusterBomClient) {
	f := func(storedClusterBom *hubv1.ClusterBom, clusterBom *hubv1.ClusterBom) bool {
		clusterbomKey := types.NamespacedName{
			Namespace: clusterBom.GetNamespace(),
			Name:      clusterBom.GetName(),
		}

		if storedClusterBom.Status.ObservedGeneration != storedClusterBom.ObjectMeta.Generation {
			Write("Status not yet changed")
			return false
		} else { // nolint
			readyCondition := getCondition(storedClusterBom, hubv1.ClusterBomReady)

			if readyCondition == nil {
				Write("ClusterBom status does not contain a condition: " + clusterbomKey.String())
				WriteClusterBom(storedClusterBom)
				os.Exit(1)
			}

			conditionStatus := readyCondition.Status
			if conditionStatus == corev1.ConditionTrue {
				if storedClusterBom.Status.OverallState != statusOk {
					Write("ClusterBom overallState is: " + storedClusterBom.Status.OverallState + " (expected ok) ")
					WriteClusterBom(storedClusterBom)
					os.Exit(1)
				}

				Write("Success: Status of ready condition is True")

				clusterReachable := getCondition(storedClusterBom, hubv1.ClusterReachable)

				if len(storedClusterBom.Spec.ApplicationConfigs) > 0 && (clusterReachable == nil || clusterReachable.Status != corev1.ConditionTrue) {
					Write("Cluster reachable condition wrong")
					WriteClusterBom(storedClusterBom)
					os.Exit(1)
				}

				Write("Success: Cluster reachable")

				Write("Status of ClusterBom successfully checked")
				return true
			} else if conditionStatus == corev1.ConditionUnknown {
				Write("Ready condition not yet True: " + conditionStatus)
				return false
			} else if conditionStatus == corev1.ConditionFalse {
				Write("Ready condition is False")
				WriteClusterBom(storedClusterBom)
				os.Exit(1)
			}

			return false
		}
	}

	CheckClusterBomGeneric(ctx, clusterBom, cl, f)
}

func getCondition(clusterbom *hubv1.ClusterBom, conditionType hubv1.ClusterBomConditionType) *hubv1.ClusterBomCondition {
	for i := range clusterbom.Status.Conditions {
		condition := &clusterbom.Status.Conditions[i]

		if condition.Type == conditionType {
			return condition
		}
	}

	return nil
}

func CheckClusterBomGeneric(ctx context.Context, clusterBom *hubv1.ClusterBom, cl *ClusterBomClient,
	op func(*hubv1.ClusterBom, *hubv1.ClusterBom) bool) *hubv1.ClusterBom {
	clusterbomKey := types.NamespacedName{
		Namespace: clusterBom.GetNamespace(),
		Name:      clusterBom.GetName(),
	}

	Write("Checking status of ClusterBom " + clusterbomKey.String())

	for i := 0; ; i++ {
		storedClusterBom := cl.GetClusterBom(ctx, clusterbomKey)

		ok := op(storedClusterBom, clusterBom)
		if ok {
			return storedClusterBom
		}

		if i > numberOfRetries {
			Write("ClusterBom does not achieve ready condition True: " + clusterbomKey.String())
			WriteClusterBom(storedClusterBom)
			os.Exit(1)
		}

		time.Sleep(time.Second * 10)
	}
}

func CheckFailedApplication(ctx context.Context, clusterBom *hubv1.ClusterBom, appConfigID string, gardenClient client.Client) {
	for i := 0; ; i++ {
		hasReachedExpectedState, storedClusterBom := hasReachedStateFailed(ctx, clusterBom, appConfigID, gardenClient)

		if hasReachedExpectedState {
			return
		}

		if i > numberOfRetries {
			Write("Application " + appConfigID + " does not achieve expected state == failed")
			WriteClusterBom(storedClusterBom)
			os.Exit(1)
		}

		time.Sleep(time.Second * 10)
	}
}

func hasReachedStateFailed(ctx context.Context, clusterBom *hubv1.ClusterBom, appConfigID string, gardenClient client.Client) (bool, *hubv1.ClusterBom) {
	clusterBomKey := types.NamespacedName{
		Namespace: clusterBom.GetNamespace(),
		Name:      clusterBom.GetName(),
	}

	var storedClusterBom hubv1.ClusterBom
	err := gardenClient.Get(ctx, clusterBomKey, &storedClusterBom)
	if err != nil {
		Write(err, "Unable to read clusterBom "+clusterBomKey.String())
		os.Exit(1)
	}

	if storedClusterBom.Status.ObservedGeneration != storedClusterBom.ObjectMeta.Generation {
		Write("Status not yet changed")
	} else {
		appState := findAppState(&storedClusterBom, appConfigID)
		if appState == nil {
			Write("Status of application " + appConfigID + " not found")
		} else if appState.DetailedState.LastOperation.State == "failed" {
			if appState.State != statusFailed || storedClusterBom.Status.OverallState != statusFailed {
				Write("Error: lastOperation.state is failed, but application state or overall state are not failed; " +
					"application state: " + appState.State + ", overall state: " + storedClusterBom.Status.OverallState)
				WriteClusterBom(&storedClusterBom)
				os.Exit(1)
			} else {
				Write("Success: states == failed as expected")

				if appState.DetailedState.Readiness.State != "ok" {
					// We expect readiness ok, despite the failed install
					Write(err, "Readiness is not ok: "+appState.DetailedState.Readiness.State)
					WriteClusterBom(&storedClusterBom)
					os.Exit(1)
				}

				return true, &storedClusterBom
			}
		} else {
			Write("LastOperation.state of clusterbom " + clusterBomKey.String() + " and appConfig " +
				appConfigID + " is: " + appState.DetailedState.LastOperation.State)
		}
	}

	return false, &storedClusterBom
}

func findAppState(clusterBom *hubv1.ClusterBom, appID string) *hubv1.ApplicationState {
	for i := range clusterBom.Status.ApplicationStates {
		appState := &clusterBom.Status.ApplicationStates[i]
		if appState.ID == appID {
			return appState
		}
	}
	return nil
}

func CheckFailedClusterBom(ctx context.Context, clusterBom *hubv1.ClusterBom, gardenClient client.Client) {
	clusterbomKey := types.NamespacedName{
		Namespace: clusterBom.GetNamespace(),
		Name:      clusterBom.GetName(),
	}

	Write("Checking status of ClusterBom " + clusterbomKey.String())

	for i := 0; ; i++ {
		var storedClusterBom hubv1.ClusterBom

		err := gardenClient.Get(ctx, clusterbomKey, &storedClusterBom)
		if err != nil {
			Write(err, "Unable to read clusterBom "+clusterbomKey.String())
			os.Exit(1)
		}

		if storedClusterBom.Status.ObservedGeneration != storedClusterBom.ObjectMeta.Generation {
			Write("Status not yet changed")
		} else {
			readyCondition := getCondition(&storedClusterBom, hubv1.ClusterBomReady)
			if readyCondition == nil {
				Write("ClusterBom status does not contain a condition: " + clusterbomKey.String())
				WriteClusterBom(&storedClusterBom)
				os.Exit(1)
			}

			conditionStatus := readyCondition.Status
			if conditionStatus == corev1.ConditionFalse {
				if storedClusterBom.Status.OverallState != statusFailed {
					Write("ClusterBom overallState is: " + storedClusterBom.Status.OverallState + " (expected failed) ")
					WriteClusterBom(&storedClusterBom)
					os.Exit(1)
				}

				Write("Success: Status of ready condition is False (as expected)")
				return
			} else if conditionStatus == corev1.ConditionUnknown {
				Write("Ready condition not yet False: " + conditionStatus)
			} else if conditionStatus == corev1.ConditionTrue {
				Write("Ready condition is True")
				WriteClusterBom(&storedClusterBom)
				os.Exit(1)
			}
		}

		if i > numberOfRetries {
			Write("ClusterBom does not achieve ready condition False: " + clusterbomKey.String())
			WriteClusterBom(&storedClusterBom)
			os.Exit(1)
		}

		time.Sleep(time.Second * 10)
	}
}

func CheckReconcileTimestampIncreased(diListBefore, diListAfter *external.DeployItemList) {
	for i := range diListBefore.Items {
		diBefore := &diListBefore.Items[i]
		diConfigBefore := &hubv1.HubDeployItemConfiguration{}
		if err := json.Unmarshal(diBefore.Spec.Configuration.Raw, diConfigBefore); err != nil {
			Write("Unmashalling deploy item before failed " + diBefore.Name)
			os.Exit(1)
		}

		id := diConfigBefore.DeploymentConfig.ID
		timeBefore := diConfigBefore.DeploymentConfig.ReconcileTime.Time

		found := false
		for j := range diListAfter.Items {
			diAfter := &diListAfter.Items[j]
			diConfigAfter := &hubv1.HubDeployItemConfiguration{}
			if err := json.Unmarshal(diAfter.Spec.Configuration.Raw, diConfigAfter); err != nil {
				Write("Unmashalling deploy item after failed " + diAfter.Name)
				os.Exit(1)
			}

			if diConfigAfter.DeploymentConfig.ID == id {
				timeAfter := diConfigAfter.DeploymentConfig.ReconcileTime.Time
				if !timeBefore.Before(timeAfter) {
					Write("Reconcile time not increased for app " + id)
					os.Exit(1)
				}

				found = true
				break
			}
		}

		if !found {
			Write("App not found " + id)
			os.Exit(1)
		}
	}
	Write("Success: reconcile time increased for all apps")
}

func CheckLastOperationTimeIncreased(ctx context.Context, clusterbomKey types.NamespacedName,
	diListBefore *external.DeployItemList, cl *ClusterBomClient) {
	Write("Checking reconcile")

	done := repeat(func() bool {
		return checkLastOperationTimeIncreasedOnce(ctx, clusterbomKey, diListBefore, cl)
	}, 24, 10*time.Second)

	if !done {
		Write("ClusterBom not reconciled after several tries " + clusterbomKey.String())
		os.Exit(1)
	}

	Write("ClusterBom successfully reconciled")
}

func checkLastOperationTimeIncreasedOnce(ctx context.Context, clusterbomKey types.NamespacedName,
	diListBefore *external.DeployItemList, cl *ClusterBomClient) bool {
	diListAfter := cl.ListDIs(ctx, &clusterbomKey)

	for i := range diListBefore.Items {
		diBefore := &diListBefore.Items[i]
		diStatusBefore := &hubv1.HubDeployItemProviderStatus{}
		if err := json.Unmarshal(diBefore.Status.ProviderStatus.Raw, diStatusBefore); err != nil {
			Write("Unmashalling deploy item status before failed " + diBefore.Name)
			os.Exit(1)
		}

		name := diBefore.Name
		timeBefore := diStatusBefore.LastOperation.Time

		found := false
		for j := range diListAfter.Items {
			diAfter := &diListAfter.Items[j]

			if diAfter.Name != name {
				continue
			}

			found = true

			diStatusAfter := &hubv1.HubDeployItemProviderStatus{}
			if err := json.Unmarshal(diAfter.Status.ProviderStatus.Raw, diStatusAfter); err != nil {
				Write("Unmashalling deploy item status after failed " + diBefore.Name)
				os.Exit(1)
			}

			timeAfter := diStatusAfter.LastOperation.Time
			if !timeBefore.Time.Before(timeAfter.Time) {
				Write("Reconcile time not yet increased for deploy item " + name)
				return false // desired state not yet reached
			}

			break
		}

		if !found {
			Write("Deploy item not found " + name)
			os.Exit(1)
		}
	}

	Write("Success: reconcile time increased for all apps")
	return true
}

func CheckKappAppDoesNotExist(ctx context.Context, clusterBomKey types.NamespacedName, configID string, gardenClient client.Client) {
	appKey := GetAppKey(clusterBomKey, configID)

	var app v1alpha1.App
	err := gardenClient.Get(ctx, appKey, &app)
	if err != nil {
		if errors.IsNotFound(err) {
			Write("Kapp app is gone " + appKey.String())
			return
		}

		Write(err, "Unable to fetch kapp app "+appKey.String())
		os.Exit(1)
	}

	Write("Error: kapp app does still exist " + appKey.String())
	os.Exit(1)
}

func GetAppKey(clusterBomKey types.NamespacedName, configID string) types.NamespacedName {
	return types.NamespacedName{
		Namespace: clusterBomKey.Namespace,
		Name:      clusterBomKey.Name + "-" + configID,
	}
}

func CheckDeployedValue(deployedValues, expectedValues map[string]interface{}) {
	for key, expectedValue := range expectedValues {
		deployedValue, ok := deployedValues[key]
		if !ok {
			Write("Deployed value " + key + " not found")
			os.Exit(1)
		}

		expectedMap, isMap := expectedValue.(map[string]interface{})
		if isMap {
			deployedMap, isAlsoMap := deployedValue.(map[string]interface{})
			if !isAlsoMap {
				Write("Deployed value " + key + " is not a map")
				os.Exit(1)
			}
			CheckDeployedValue(deployedMap, expectedMap)
		} else { // nolint
			if !reflect.DeepEqual(deployedValue, expectedValue) {
				Write("Deployed parameter " + key + " has wrong value")
				os.Exit(1)
			}
		}
	}
}
