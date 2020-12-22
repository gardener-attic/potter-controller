package deployutil

import (
	"context"
)

type DeployItemDeployer interface {
	ProcessNewOperation(ctx context.Context, deployData *DeployData)
	RetryFailedOperation(ctx context.Context, deployData *DeployData)
	ReconcileOperation(ctx context.Context, deployData *DeployData)
	ProcessPendingOperation(ctx context.Context, deployData *DeployData)
	Cleanup(ctx context.Context, deployData *DeployData, clusterExists bool) error
}
