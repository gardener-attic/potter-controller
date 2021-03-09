package synchronize

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"context"
)

// TODO
// check reconciler
// remove own client for admission hook

type UncachedClient interface {
	GetUncached(ctx context.Context, key types.NamespacedName, obj client.Object) error
	ListUncached(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
	Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
	Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
}

func NewUncachedClient(config *rest.Config, options client.Options) (UncachedClient, error) {
	clt, err := client.New(config, options)
	if err != nil {
		return nil, err
	}

	return &uncachedClientImpl{uncachedClient: clt}, nil
}

type uncachedClientImpl struct {
	uncachedClient client.Client
}

func (r *uncachedClientImpl) GetUncached(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	return r.uncachedClient.Get(ctx, key, obj)
}

func (r *uncachedClientImpl) ListUncached(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return r.uncachedClient.List(ctx, list, opts...)
}

func (r *uncachedClientImpl) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return r.uncachedClient.Create(ctx, obj, opts...)
}

func (r *uncachedClientImpl) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return r.uncachedClient.Update(ctx, obj, opts...)
}

func (r *uncachedClientImpl) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return r.uncachedClient.Delete(ctx, obj, opts...)
}
