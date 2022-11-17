package controllersdi

import (
	"context"
	"github.com/gardener/potter-controller/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type clusterbomDeactivator struct{}

func (d *clusterbomDeactivator) handleDeactivationOrReactivation(ctx context.Context, associatedObjects *AssociatedObjects,
	r client.Client) (stopReconcile, actionProgressing bool, err error) {
	if util.HasAnnotation(&associatedObjects.clusterbom, util.AnnotationActionIgnoreKey, util.Deactivate) {
		return d.deactivate(ctx, associatedObjects, r)

	} else if util.HasAnnotation(&associatedObjects.clusterbom, util.AnnotationActionIgnoreKey, util.Reactivate) {
		return d.reactivate(ctx, associatedObjects, r)

	} else if util.HasAnnotation(&associatedObjects.clusterbom, util.AnnotationStatusIgnoreKey, util.Ignore) {
		// Is deactivated
		return true, false, nil
	}

	// Not deactivated, the normal reconcile should be done
	return false, false, nil
}

func (d *clusterbomDeactivator) deactivate(ctx context.Context, associatedObjects *AssociatedObjects, r client.Client) (stopReconcile, actionProgressing bool, err error) {
	allItemsDeactivated := true

	// Forward action to all items
	for k, _ := range associatedObjects.deployItemList.Items {
		item := &associatedObjects.deployItemList.Items[k]
		if util.HasAnnotation(item, util.AnnotationStatusIgnoreKey, util.Ignore) &&
			!util.HasAnnotation(item, util.AnnotationActionIgnoreKey, util.Deactivate) &&
			!util.HasAnnotation(item, util.AnnotationActionIgnoreKey, util.Reactivate) {
			// Item is deactivated
			continue
		} else if util.HasAnnotation(item, util.AnnotationActionIgnoreKey, util.Deactivate) {
			// Item is informed about deactivation, but has not yet confirmed
			allItemsDeactivated = false
		} else {
			// Item needs to be informed about deactivation
			util.AddAnnotation(item, util.AnnotationActionIgnoreKey, util.Deactivate)
			allItemsDeactivated = false
			if err = r.Update(ctx, item); err != nil {
				return false, false, err
			}
		}
	}

	if allItemsDeactivated {
		// Mark clusterbom as deactivated
		util.RemoveAnnotation(&associatedObjects.clusterbom, util.AnnotationActionIgnoreKey)
		util.AddAnnotation(&associatedObjects.clusterbom, util.AnnotationStatusIgnoreKey, util.Ignore)
		if err = r.Update(ctx, &associatedObjects.clusterbom); err != nil {
			return false, false, err
		}
		// Deactivation done
		return true, false, nil

	} else {
		// Deactivation is progressing, re-check necessary
		return false, true, nil
	}
}

func (d *clusterbomDeactivator) reactivate(ctx context.Context, associatedObjects *AssociatedObjects, r client.Client) (stopReconcile, actionProgressing bool, err error) {
	allItemsReactivated := true

	for k, _ := range associatedObjects.deployItemList.Items {
		item := &associatedObjects.deployItemList.Items[k]
		if !util.HasAnnotation(item, util.AnnotationStatusIgnoreKey, util.Ignore) &&
			!util.HasAnnotation(item, util.AnnotationActionIgnoreKey, util.Deactivate) &&
			!util.HasAnnotation(item, util.AnnotationActionIgnoreKey, util.Reactivate) {
			// Item is reactivated
			continue
		} else if util.HasAnnotation(item, util.AnnotationActionIgnoreKey, util.Reactivate) {
			// Item is informed about reactivation, but has not yet confirmed
			allItemsReactivated = false
		} else {
			// Item needs to be informed about reactivation
			util.AddAnnotation(item, util.AnnotationActionIgnoreKey, util.Reactivate)
			allItemsReactivated = false
			if err = r.Update(ctx, item); err != nil {
				return false, false, err
			}
		}
	}

	if allItemsReactivated {
		util.RemoveAnnotation(&associatedObjects.clusterbom, util.AnnotationActionIgnoreKey)
		util.RemoveAnnotation(&associatedObjects.clusterbom, util.AnnotationStatusIgnoreKey)
		if err = r.Update(ctx, &associatedObjects.clusterbom); err != nil {
			return false, false, err
		}
		// Reactivation done (nevertheless we stop the current reconcile here)
		return true, false, nil

	} else {
		// Reactivation is progressing, re-check necessary
		return false, true, nil
	}
}

func (d *clusterbomDeactivator) deleteIfRequired(ctx context.Context, associatedObjects *AssociatedObjects, r client.Client) error {
	clusterbom := associatedObjects.clusterbom
	if clusterbom.ObjectMeta.DeletionTimestamp != nil &&
		util.HasAnnotation(&clusterbom, util.AnnotationStatusIgnoreKey, util.Ignore) &&
		!util.HasAnnotation(&clusterbom, util.AnnotationActionIgnoreKey, util.Deactivate) &&
		!util.HasAnnotation(&clusterbom, util.AnnotationActionIgnoreKey, util.Reactivate) {
		for k, _ := range associatedObjects.deployItemList.Items {
			item := &associatedObjects.deployItemList.Items[k]
			item.SetFinalizers(nil)
			if err := r.Update(ctx, item); err != nil {
				return err
			}
			if err := r.Delete(ctx, item); err != nil {
				return err
			}
		}

		clusterbom.SetFinalizers(nil)
		if err := r.Update(ctx, &clusterbom); err != nil {
			return err
		}
	}

	return nil
}
