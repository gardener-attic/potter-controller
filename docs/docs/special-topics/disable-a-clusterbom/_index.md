---
title: Disable a ClusterBom
type: docs
---

It is possible to disable a ClusterBom. This means that nothing is deployed by the configured application until
the ClusterBom is reactivated again. 

### Deactivate ClusterBom 

You deactivate a ClusterBom by setting the annotation 
`potter.gardener.cloud/action-ignore: deactivate` at the ClusterBom. The potter controller waits until all currently
running deployments are finished and stops all future ones. Then it replaces the annotation
`potter.gardener.cloud/action-ignore: deactivate` by the annotation
`potter.gardener.cloud/status-ignore: ignore`. This signals the user that all no deployments are running and no new 
ones will be started.

If you set again the annotation `potter.gardener.cloud/action-ignore: deactivate` in such a situation, this makes no 
problem and the annotation is removed immediately. This allows you to statically add this annotation to your manifest, 
as long as you want to have this ClusterBom deactivated.

**Warning:** Do not modify the annotation `potter.gardener.cloud/status-ignore: ignore` by yourself. This must be set 
and removed by the potter controller only.

### Reactivate a Deactivated ClusterBom

A deactivated ClusterBom could be reactivated setting the annotation
`potter.gardener.cloud/action-ignore: reactivate` at the ClusterBom. When this operation was successfully finished,
the potter controller removes this annotation and also the annotation `potter.gardener.cloud/status-ignore: ignore`.

### Deleting a Deactivated ClusterBom without deleting the deployed applications

When you delete a deactivated ClusterBom nothing is uninstalled from the target cluster.