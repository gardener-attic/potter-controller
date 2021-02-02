package util

import (
	"context"
	"os"
	"time"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"

	landscaper "github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	statusOk     = "ok"
	statusFailed = "failed"
)

type ClusterBomClient struct {
	gardenClient client.Client
}

func NewClusterBomClient(gardenClient client.Client) *ClusterBomClient {
	return &ClusterBomClient{gardenClient: gardenClient}
}

func (c *ClusterBomClient) GetClusterBom(ctx context.Context, clusterBomKey types.NamespacedName) *hubv1.ClusterBom {
	var clusterBom hubv1.ClusterBom

	err := c.gardenClient.Get(ctx, clusterBomKey, &clusterBom)
	if err != nil {
		Write(err, "Unable to read clusterbom "+clusterBomKey.String())
		os.Exit(1)
	}

	return &clusterBom
}

func (c *ClusterBomClient) CreateClusterBom(ctx context.Context, clusterBom *hubv1.ClusterBom) {
	clusterBomKey := types.NamespacedName{
		Namespace: clusterBom.GetNamespace(),
		Name:      clusterBom.GetName(),
	}

	Write("Creating clusterbom " + clusterBomKey.String())

	err := c.gardenClient.Create(ctx, clusterBom)
	if err != nil {
		Write(err, "Error creating clusterbom "+clusterBomKey.String())
		WriteClusterBom(clusterBom)
		os.Exit(1)
	}

	Write("Clusterbom successfully created")
}

// ListHDCs lists all deploy items associated with a clusterbom
func (c *ClusterBomClient) ListDIs(ctx context.Context, clusterBomKey *types.NamespacedName) *landscaper.DeployItemList {
	diList := landscaper.DeployItemList{}
	err := c.gardenClient.List(ctx, &diList, client.InNamespace(clusterBomKey.Namespace), client.MatchingLabels{LabelClusterBomName: clusterBomKey.Name})
	if err != nil {
		Write(err, "Error listing deployitems for clusterbom "+clusterBomKey.String())
		os.Exit(1)
	}
	return &diList
}

func (c *ClusterBomClient) UpdateAppConfigsInClusterBom(ctx context.Context, clusterBom *hubv1.ClusterBom) metav1.Time {
	setAppConfigs := func(storedClusterBom, clusterBom *hubv1.ClusterBom) {
		storedClusterBom.Spec.ApplicationConfigs = clusterBom.Spec.ApplicationConfigs
	}

	return updateClusterBomGeneric(ctx, clusterBom, c.gardenClient, setAppConfigs)
}

func (c *ClusterBomClient) AddReconcileAnnotationInClusterBom(ctx context.Context, clusterBom *hubv1.ClusterBom) metav1.Time {
	addAnnotation := func(storedClusterBom, clusterBom *hubv1.ClusterBom) {
		if storedClusterBom.Annotations == nil {
			storedClusterBom.Annotations = make(map[string]string)
		}
		storedClusterBom.Annotations["hub.k8s.sap.com/reconcile"] = "reconcile"
	}

	return updateClusterBomGeneric(ctx, clusterBom, c.gardenClient, addAnnotation)
}

func updateClusterBomGeneric(ctx context.Context, clusterBom *hubv1.ClusterBom, gardenClient client.Client,
	op func(*hubv1.ClusterBom, *hubv1.ClusterBom)) metav1.Time {
	clusterbomKey := types.NamespacedName{
		Namespace: clusterBom.GetNamespace(),
		Name:      clusterBom.GetName(),
	}

	Write("Updating ClusterBom " + clusterbomKey.String())

	for i := 0; ; i++ {
		Write("Updating...")
		var storedClusterBom hubv1.ClusterBom
		err := gardenClient.Get(ctx, clusterbomKey, &storedClusterBom)

		if err != nil {
			Write(err, "Error fetching stored clusterbom "+clusterbomKey.String())
			os.Exit(1)
		}

		op(&storedClusterBom, clusterBom)

		storedClusterBom.Spec.ApplicationConfigs = clusterBom.Spec.ApplicationConfigs

		err = gardenClient.Update(ctx, &storedClusterBom)

		if err != nil {
			switch v := err.(type) {
			case *errors.StatusError:
				if v.ErrStatus.Code == 409 {
					Write("Conflict updating clusterbom " + clusterbomKey.String())
				} else {
					Write(err, "Error updating clusterbom "+clusterbomKey.String())
					WriteClusterBom(&storedClusterBom)
					os.Exit(1)
				}
			default:
				Write(err, "Error updating clusterbom "+clusterbomKey.String())
				WriteClusterBom(&storedClusterBom)
				os.Exit(1)
			}
		} else {
			Write("ClusterBom successfully updated")
			return storedClusterBom.Status.OverallTime
		}
	}
}

func (c *ClusterBomClient) DeleteClusterBomAsync(ctx context.Context, clusterBomKey types.NamespacedName) chan bool {
	finished := make(chan bool)
	go func() {
		c.DeleteClusterBom(ctx, clusterBomKey)
		finished <- true
	}()
	return finished
}

func (c *ClusterBomClient) DeleteClusterBom(ctx context.Context, clusterBomKey types.NamespacedName) {
	Write("Deleting ClusterBom " + clusterBomKey.String())

	done := repeat(func() bool {
		return deleteClusterBomOnce(ctx, c.gardenClient, clusterBomKey)
	}, 24, 10*time.Second)

	if !done {
		Write("Unable to delete clusterBom after several tries " + clusterBomKey.String())
		os.Exit(1)
	}

	Write("ClusterBom successfully deleted")
}

func deleteClusterBomOnce(ctx context.Context, gardenClient client.Client, clusterBomKey types.NamespacedName) bool {
	Write("Trying to delete clusterbom " + clusterBomKey.String())

	var clusterBom hubv1.ClusterBom
	err := gardenClient.Get(ctx, clusterBomKey, &clusterBom)
	if err != nil {
		if errors.IsNotFound(err) {
			return true
		}

		Write(err, "Unable to read clusterBom "+clusterBomKey.String())
		os.Exit(1)
	} else {
		Write("Call delete clusterbom " + clusterBomKey.String())
		err = gardenClient.Delete(ctx, &clusterBom)
		if err != nil {
			err2 := gardenClient.Get(ctx, clusterBomKey, &clusterBom)
			if errors.IsNotFound(err2) {
				return true
			}

			Write(err, "Unable to delete clusterBom "+clusterBomKey.String())
			os.Exit(1)
		}
	}

	return false
}

func GetDeployItems(ctx context.Context, gardenClient client.Client,
	clusterBomKey types.NamespacedName, numOfDeployItems int) *landscaper.DeployItemList {
	deployItemList := landscaper.DeployItemList{}

	op := func() bool {
		deployItemList.Items = []landscaper.DeployItem{}

		err := gardenClient.List(ctx, &deployItemList, client.InNamespace(clusterBomKey.Namespace),
			client.MatchingLabels{hubv1.LabelClusterBomName: clusterBomKey.Name})

		if err != nil {
			Write(err, "Unable to read deploy items")
			return false
		}

		if len(deployItemList.Items) == numOfDeployItems {
			return true
		}

		return false
	}

	done := repeat(op, 5, 5*time.Second)

	if !done {
		Write("Unable to delete get deploy items after several tries " + clusterBomKey.String())
		os.Exit(1)
	}

	Write("Deploy Items fetched")

	return &deployItemList
}

func Repeat(f func() bool, repetitions int, pause time.Duration) bool {
	return repeat(f, repetitions, pause)
}

func repeat(f func() bool, repetitions int, pause time.Duration) bool {
	for i := 0; i < repetitions; i++ {
		if i > 0 {
			time.Sleep(pause)
		}

		done := f()
		if done {
			return true
		}
	}
	return false
}

func Parallel(fs ...func()) {
	c := make(chan bool)

	for i := range fs {
		go func(index int, done chan bool) {
			fs[index]()
			done <- true
		}(i, c)
	}

	for range fs {
		<-c
	}
}
