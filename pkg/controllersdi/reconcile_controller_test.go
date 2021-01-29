package controllersdi

import (
	"context"
	"testing"
	"time"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	hubtesting "github.wdf.sap.corp/kubernetes/hub-controller/pkg/testing"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"

	"github.com/arschles/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	testReconcileMapName      = "testReconcileMapName"
	testReconcileMapNamespace = "testReconcileMapNamespace"
)

func TestHandleNoDeploymentConfigs(t *testing.T) {
	interval := 1 * time.Hour
	configMapKey := types.NamespacedName{Name: testReconcileMapName, Namespace: testReconcileMapNamespace}

	unitTestClient := hubtesting.NewUnitTestClientDi()
	hubControllerTestClient := hubtesting.HubControllerTestClientDi{}

	r := ReconcileController{
		Client:              unitTestClient,
		HubControllerClient: hubControllerTestClient,
		Log:                 ctrl.Log.WithName("controllers").WithName("ReconcileController"),
		Scheme:              runtime.NewScheme(),
		Clock:               &RealReconcileClock{},
		ConfigMapKey:        configMapKey,
	}

	log := r.Log.WithValues(
		util.LogKeyInterval, interval.String(),
		util.LogKeyConfigmap, configMapKey,
	)
	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, log)

	r.handleAllClusterBoms(ctx)

	assert.Equal(t, len(unitTestClient.DeployItems), 0, "number of deploy items")
}

func TestAutoDelete(t *testing.T) {
	log := ctrl.Log.WithName("controllers").WithName("ReconcileController")
	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, log)

	const (
		namespace      = "testNamespace"
		clusterBomName = "testClusterBom"
		secretName     = "testCluster.kubeconfig"
	)

	creationTime := time.Date(2020, time.March, 1, 14, 0, 0, 0, time.UTC)
	now := creationTime.Add(60 * time.Minute)

	tests := []struct {
		name               string
		autoDeleteAge      int64
		clusterExists      bool
		autoDeleteExpected bool
	}{
		{
			name:               "autodelete",
			autoDeleteAge:      59,
			clusterExists:      false,
			autoDeleteExpected: true,
		},
		{
			name:               "cluster exists",
			autoDeleteAge:      59,
			clusterExists:      true,
			autoDeleteExpected: false,
		},
		{
			name:               "no autodelete config",
			autoDeleteAge:      0,
			clusterExists:      false,
			autoDeleteExpected: false,
		},
		{
			name:               "clusterbom too young",
			autoDeleteAge:      61,
			clusterExists:      false,
			autoDeleteExpected: false,
		},
	}

	for i := range tests {
		test := &tests[i]
		t.Run(test.name, func(t *testing.T) {
			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
			}

			clusterBom := hubv1.ClusterBom{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterBomName,
					Namespace: namespace,
					CreationTimestamp: metav1.Time{
						Time: creationTime,
					},
				},
				Spec: hubv1.ClusterBomSpec{
					AutoDelete: &hubv1.AutoDelete{
						ClusterBomAge: test.autoDeleteAge,
					},
					SecretRef: secret.Name,
				},
			}

			unitTestClient := hubtesting.NewUnitTestClientDi()
			unitTestClient.AddClusterBom(&clusterBom)
			if test.clusterExists {
				unitTestClient.AddSecret(&secret)
			}

			r := ReconcileController{
				Client: unitTestClient,
				Log:    log,
				Scheme: runtime.NewScheme(),
				Clock:  &hubtesting.FakeReconcileClockDi{Time: now},
			}

			clusterBomKey := util.GetKey(&clusterBom)

			r.autoDeleteClusterlessBom(ctx, clusterBomKey)

			var clusterBomAfterAutoDelete hubv1.ClusterBom
			err := unitTestClient.Get(ctx, *clusterBomKey, &clusterBomAfterAutoDelete)
			if test.autoDeleteExpected {
				assert.NotNil(t, err, "error")
			} else {
				assert.Nil(t, err, "error")
				assert.NotNil(t, clusterBomAfterAutoDelete, "clusterbom")
			}
		})
	}
}
