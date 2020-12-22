---
title: Manual Reconcile
type: docs
---

You can trigger a reconcile for a Cluster-BoM by adding the following annotation. If this annotation is set, a reconcile loop is triggered for all apps of the Cluster-BoM except those for which automatic reconcile is disabled. The annotation is removed automatically after this operation.

```yaml
metadata:
  annotations:
    hub.k8s.sap.com/reconcile: reconcile
```