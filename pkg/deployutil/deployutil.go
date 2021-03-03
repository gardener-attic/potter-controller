package deployutil

import (
	"context"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/util"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/api/apps/v1beta1"
	"k8s.io/api/apps/v1beta2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func isResourceReady(ctx context.Context, resource runtime.Object) bool {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	switch typedResource := resource.(type) {
	// Readiness for different versions of DaemonSet
	case *v1.DaemonSet:
		return compareNumbers(ctx, typedResource.Status.NumberReady, typedResource.Status.CurrentNumberScheduled)
	case *extensions.DaemonSet:
		return compareNumbers(ctx, typedResource.Status.NumberReady, typedResource.Status.CurrentNumberScheduled)
	case *v1beta2.DaemonSet:
		return compareNumbers(ctx, typedResource.Status.NumberReady, typedResource.Status.CurrentNumberScheduled)

	// Readiness for different versions of Deployment
	case *v1.Deployment:
		return checkIfUpToDate(ctx, typedResource.Generation, typedResource.Status.ObservedGeneration,
			typedResource.Spec.Replicas, typedResource.Status.UpdatedReplicas, typedResource.Status.Replicas,
			typedResource.Status.AvailableReplicas)
	case *v1beta1.Deployment:
		return checkIfUpToDate(ctx, typedResource.Generation, typedResource.Status.ObservedGeneration,
			typedResource.Spec.Replicas, typedResource.Status.UpdatedReplicas, typedResource.Status.Replicas,
			typedResource.Status.AvailableReplicas)
	case *v1beta2.Deployment:
		return checkIfUpToDate(ctx, typedResource.Generation, typedResource.Status.ObservedGeneration,
			typedResource.Spec.Replicas, typedResource.Status.UpdatedReplicas, typedResource.Status.Replicas,
			typedResource.Status.AvailableReplicas)
	case *extensions.Deployment:
		return checkIfUpToDate(ctx, typedResource.Generation, typedResource.Status.ObservedGeneration,
			typedResource.Spec.Replicas, typedResource.Status.UpdatedReplicas, typedResource.Status.Replicas,
			typedResource.Status.AvailableReplicas)

	// Readiness for different versions of StatefulSet
	case *v1.StatefulSet:
		return compareNumbers(ctx, typedResource.Status.ReadyReplicas, typedResource.Status.Replicas)
	case *v1beta1.StatefulSet:
		return compareNumbers(ctx, typedResource.Status.ReadyReplicas, typedResource.Status.Replicas)
	case *v1beta2.StatefulSet:
		return compareNumbers(ctx, typedResource.Status.ReadyReplicas, typedResource.Status.Replicas)

	default:
		log.Error(nil, "Unknown resource type")
		return false
	}
}

func checkIfUpToDate(ctx context.Context, generation, observerGeneration int64, specReplicasPointer *int32,
	statusUpdatedReplicas, statusReplicas, statusAvailableReplicas int32) bool {
	log := util.GetLoggerFromContext(ctx)

	log.V(util.LogLevelDebug).Info("compare for readiness",
		"generation", generation,
		"observerGeneration", observerGeneration,
		"specReplicasPointer", specReplicasPointer,
		"statusUpdatedReplicas", statusUpdatedReplicas,
		"statusReplicas", statusReplicas,
		"statusAvailableReplicas", statusAvailableReplicas)

	var specReplicas int32 = 1
	if specReplicasPointer != nil {
		specReplicas = *specReplicasPointer
	}
	return (generation == observerGeneration && specReplicas == statusUpdatedReplicas &&
		specReplicas == statusReplicas && specReplicas == statusAvailableReplicas)
}

func compareNumbers(ctx context.Context, num1, num2 int32) bool {
	log := util.GetLoggerFromContext(ctx)

	log.V(util.LogLevelDebug).Info("compare for readiness", "num1", num1, "num2", num2)

	return (num1 != 0) && (num1 == num2)
}

func readFromTargetCluster(ctx context.Context, cl client.Client, object *BasicKubernetesObject) (runtime.Object, error) {
	log := util.GetLoggerFromContext(ctx)

	key := types.NamespacedName{Namespace: object.ObjectMeta.Namespace, Name: object.ObjectMeta.Name}

	var result runtime.Object
	var err error

	if object.Kind == util.KindDaemonSet {
		if object.APIVersion == util.APIVersionAppsV1 {
			var tmp v1.DaemonSet
			err = cl.Get(ctx, key, &tmp)
			result = &tmp
		} else if object.APIVersion == util.APIVersionExtensionsV1beta1 {
			var tmp extensions.DaemonSet
			err = cl.Get(ctx, key, &tmp)
			result = &tmp
		} else if object.APIVersion == util.APIVersionAppsV1beta2 {
			var tmp v1beta2.DaemonSet
			err = cl.Get(ctx, key, &tmp)
			result = &tmp
		} else {
			err = errors.New("unknown version")
			log.Error(err, err.Error())
		}
	} else if object.Kind == util.KindDeployment {
		if object.APIVersion == util.APIVersionAppsV1 {
			var tmp v1.Deployment
			err = cl.Get(ctx, key, &tmp)
			result = &tmp
		} else if object.APIVersion == util.APIVersionAppsV1beta1 {
			var tmp v1beta1.Deployment
			err = cl.Get(ctx, key, &tmp)
			result = &tmp
		} else if object.APIVersion == util.APIVersionAppsV1beta2 {
			var tmp v1beta2.Deployment
			err = cl.Get(ctx, key, &tmp)
			result = &tmp
		} else if object.APIVersion == util.APIVersionExtensionsV1beta1 {
			var tmp extensions.Deployment
			err = cl.Get(ctx, key, &tmp)
			result = &tmp
		} else {
			err = errors.New("unknown version")
			log.Error(err, err.Error())
		}
	} else if object.Kind == util.KindStatefulSet {
		if object.APIVersion == util.APIVersionAppsV1beta1 {
			var tmp v1beta1.StatefulSet
			err = cl.Get(ctx, key, &tmp)
			result = &tmp
		} else if object.APIVersion == util.APIVersionAppsV1beta2 {
			var tmp v1beta2.StatefulSet
			err = cl.Get(ctx, key, &tmp)
			result = &tmp
		} else if object.APIVersion == util.APIVersionAppsV1 {
			var tmp v1.StatefulSet
			err = cl.Get(ctx, key, &tmp)
			result = &tmp
		} else {
			err = errors.New("unknown version")
			log.Error(err, err.Error())
		}
	} else if object.Kind == util.KindJob {
		if object.APIVersion == util.APIVersionBatchV1 {
			var tmp batchv1.Job
			err = cl.Get(ctx, key, &tmp)
			result = &tmp
		} else {
			err = errors.New("unknown version")
			log.Error(err, err.Error())
		}
	} else {
		err = errors.New("unknown kind")
		log.Error(err, err.Error())
	}

	return result, err
}

func WorseState(state1, state2 string) string {
	if state1 == util.StateFinallyFailed || state2 == util.StateFinallyFailed {
		return util.StateFinallyFailed
	} else if state1 == util.StateFailed || state2 == util.StateFailed {
		return util.StateFailed
	} else if state1 == util.StateUnknown || state2 == util.StateUnknown {
		return util.StateUnknown
	} else if state1 == util.StatePending || state2 == util.StatePending {
		return util.StatePending
	}

	return util.StateOk
}

func GetTargetClient(ctx context.Context, crAndSecretClient client.Client, secretKey types.NamespacedName) (client.Client, error) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	targetKubeconfig, err := GetTargetConfig(ctx, crAndSecretClient, secretKey)
	if err != nil {
		return nil, err
	}

	restClientConfig, err := clientcmd.RESTConfigFromKubeConfig(targetKubeconfig)
	if err != nil {
		log.Error(err, "Error creating clientconfig for target cluster")
		return nil, err
	}

	clt, err := client.New(restClientConfig, client.Options{})
	if err != nil {
		log.Error(err, "Could not create client for target cluster")
		return nil, &ClusterUnreachableError{Err: err}
	}

	return clt, nil
}

func DoesTargetClusterExist(ctx context.Context, crAndSecretClient client.Client, secretKey types.NamespacedName) (bool, error) {
	_, err := GetTargetSecret(ctx, crAndSecretClient, secretKey)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		return true, err
	}

	return true, nil
}

func GetTargetConfig(ctx context.Context, crAndSecretClient client.Client, secretKey types.NamespacedName) ([]byte, error) {
	kubeconfigSecret, err := GetTargetSecret(ctx, crAndSecretClient, secretKey)
	if err != nil {
		return nil, err
	}

	targetKubeconfig := kubeconfigSecret.Data["kubeconfig"]

	return targetKubeconfig, nil
}

func GetTargetSecret(ctx context.Context, crAndSecretClient client.Client, secretKey types.NamespacedName) (*corev1.Secret, error) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	kubeconfigSecret := &corev1.Secret{}

	err := crAndSecretClient.Get(ctx, secretKey, kubeconfigSecret)
	if err != nil {
		log.Error(err, "Could not fetch secret for target cluster")
		return nil, err
	}

	return kubeconfigSecret, nil
}

func MergeMaps(a, b map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(a))

	// copy first map to result map
	for key, value := range a {
		result[key] = value
	}

	// merge second map into result map
	for key, value := range b {
		valueMap, isMap := value.(map[string]interface{})
		if isMap {
			if bv, ok := result[key]; ok {
				if bv, ok := bv.(map[string]interface{}); ok {
					// if a key occurs in both maps and both values are maps, then merge them recursively
					result[key] = MergeMaps(bv, valueMap)
					continue
				}
			}
		}

		result[key] = value
	}

	return result
}

type ClusterUnreachableError struct {
	Err error
}

func (e *ClusterUnreachableError) Error() string {
	return e.Err.Error()
}

func ComputeReadinessForResourceReadyRequirements(resourceReadyRequirements []hubv1.Resource,
	resultReadiness string, dynamicClient *DynamicTargetClient, logger logr.Logger) string {
	for i, resourceReadyRequirement := range resourceReadyRequirements {
		obj := BasicKubernetesObject{
			ObjectMeta: types.NamespacedName{
				Namespace: resourceReadyRequirement.Namespace,
				Name:      resourceReadyRequirement.Name,
			},
			APIVersion: resourceReadyRequirement.APIVersion,
			Kind:       resourceReadyRequirement.Resource,
		}

		resource, err := dynamicClient.GetResource(resourceReadyRequirement.APIVersion, resourceReadyRequirement.Resource,
			resourceReadyRequirement.Namespace, resourceReadyRequirement.Name)

		loggerForObject := logger.
			WithValues("object", obj).
			WithValues("resourceReadyRequirementIndex", i)

		if err != nil {
			loggerForObject.Error(err, "Error reading resource from target cluster")
			resultReadiness = WorseState(resultReadiness, util.StateUnknown)
			continue
		}

		successValues, err := util.ParseSuccessValues(resourceReadyRequirement.SuccessValues)
		if err != nil {
			loggerForObject.Error(err, "Cannot parse successValues")
			resultReadiness = WorseState(resultReadiness, util.StateUnknown)
			continue
		}

		results, err := util.GetFieldsByJSONPath(resource.Object, resourceReadyRequirement.FieldPath)
		if err != nil {
			loggerForObject.Error(err, "Cannot get fields by fieldPath")
			resultReadiness = WorseState(resultReadiness, util.StateUnknown)
			continue
		}

		for _, result := range results {
			for _, valueFromResource := range result {
				if !util.ContainsValue(valueFromResource.Interface(), successValues) {
					resultReadiness = WorseState(resultReadiness, util.StateUnknown)
					continue
				}
			}
		}
	}

	return resultReadiness
}
