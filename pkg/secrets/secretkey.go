package secrets

import (
	"context"
	"sync"

	"github.com/gardener/potter-controller/pkg/synchronize"
	"github.com/gardener/potter-controller/pkg/util"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	secretDeletionKey []byte     // nolint
	globalMutex       sync.Mutex // nolint
)

func GetSecretDeletionKey(ctx context.Context, clt synchronize.UncachedClient, fromCache bool) ([]byte, error) {
	globalMutex.Lock()
	defer globalMutex.Unlock()

	if secretDeletionKey == nil || !fromCache {
		secretKey := types.NamespacedName{
			Namespace: util.GetPodNamespace(),
			Name:      "secret-private-keys",
		}

		var secret v1.Secret
		err := clt.GetUncached(ctx, secretKey, &secret)
		if err != nil {
			return nil, err
		}

		secretDeletionKey = secret.Data["secretDeletionKey"]
	}

	return secretDeletionKey, nil
}
