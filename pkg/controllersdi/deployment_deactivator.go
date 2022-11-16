package controllersdi

import (
	"context"
	"github.com/gardener/landscaper/apis/core/v1alpha1"
	"github.com/gardener/potter-controller/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type deploymentDeactivator struct{}

func (d *deploymentDeactivator) handleDeactivationOrReactivation(ctx context.Context, deployItem *v1alpha1.DeployItem, r client.Client) (stopReconcile bool, err error) {
	if util.HasAnnotation(deployItem, util.AnnotationActionIgnoreKey, util.Deactivate) {
		return d.deactivate(ctx, deployItem, r)

	} else if util.HasAnnotation(deployItem, util.AnnotationActionIgnoreKey, util.Reactivate) {
		return d.reactivate(ctx, deployItem, r)

	} else if util.HasAnnotation(deployItem, util.AnnotationStatusIgnoreKey, util.Ignore) {
		// Is deactivated
		return true, nil
	}

	// Not deactivated, the normal reconcile should be done
	return false, nil
}

func (d *deploymentDeactivator) deactivate(ctx context.Context, deployItem *v1alpha1.DeployItem, r client.Client) (stopReconcile bool, err error) {
	util.RemoveAnnotation(deployItem, util.AnnotationActionIgnoreKey)
	util.AddAnnotation(deployItem, util.AnnotationStatusIgnoreKey, util.Ignore)
	if err = r.Update(ctx, deployItem); err != nil {
		return false, err
	}
	// Deactivation done
	return true, nil
}

func (d *deploymentDeactivator) reactivate(ctx context.Context, deployItem *v1alpha1.DeployItem, r client.Client) (stopReconcile bool, err error) {
	util.RemoveAnnotation(deployItem, util.AnnotationActionIgnoreKey)
	util.RemoveAnnotation(deployItem, util.AnnotationStatusIgnoreKey)
	if err = r.Update(ctx, deployItem); err != nil {
		return false, err
	}
	// Reactivation done (nevertheless we stop the current reconcile here)
	return true, nil
}
