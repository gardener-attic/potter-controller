package deployutil

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gardener/potter-controller/pkg/util"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DynamicTargetClient struct {
	client dynamic.Interface
}

func NewDynamicTargetClient(ctx context.Context, crAndSecretClient client.Client, secretKey types.NamespacedName) (*DynamicTargetClient, error) {
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

	dynamicClient, err := dynamic.NewForConfig(restClientConfig)
	if err != nil {
		log.Error(err, "Could not create client for target cluster")
		return nil, &ClusterUnreachableError{Err: err}
	}

	return &DynamicTargetClient{
		client: dynamicClient,
	}, nil
}

func (d *DynamicTargetClient) GetResource(apiVersion, resource, namespace, name string) (*unstructured.Unstructured, error) {
	splittedAPIVersion := strings.Split(apiVersion, "/")
	var gvr schema.GroupVersionResource
	if len(splittedAPIVersion) == 1 {
		gvr = schema.GroupVersionResource{
			Version:  splittedAPIVersion[0],
			Resource: resource,
		}
	} else {
		gvr = schema.GroupVersionResource{
			Group:    splittedAPIVersion[0],
			Version:  splittedAPIVersion[1],
			Resource: resource,
		}
	}

	return d.client.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

func (d *DynamicTargetClient) GetResourceData(apiVersion, resource, namespace, name, fieldPath string) (interface{}, error) {
	unstructuredObject, err := d.GetResource(apiVersion, resource, namespace, name)

	if err != nil {
		return nil, err
	}

	return d.getFieldByJSONPath(unstructuredObject, fieldPath)
}

func (d *DynamicTargetClient) getFieldByJSONPath(resource *unstructured.Unstructured, fieldPath string) (interface{}, error) {
	if !strings.HasPrefix(fieldPath, ".") {
		fieldPath = "." + fieldPath
	}

	jp := jsonpath.New("get")
	if err := jp.Parse(fmt.Sprintf("{%s}", fieldPath)); err != nil {
		return nil, err
	}

	res, err := jp.FindResults(resource.Object)
	if err != nil {
		return nil, err
	}

	if len(res) == 0 {
		return nil, errors.New("not found")
	}

	if len(res) != 1 && len(res[0]) != 1 {
		return nil, errors.New("expected exactly one result")
	}

	return res[0][0].Interface(), nil
}
