package apitypes

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NamedSecretResolver struct {
	secretClient             client.Client
	secretNamespace          string
	namedInternalSecretNames map[string]string
}

func NewNamedSecretResolver(secretClient client.Client, secretNamespace string,
	namedInternalSecretNames map[string]string) *NamedSecretResolver {
	return &NamedSecretResolver{
		secretClient:             secretClient,
		secretNamespace:          secretNamespace,
		namedInternalSecretNames: namedInternalSecretNames,
	}
}

func (r *NamedSecretResolver) ResolveSecretValue(ctx context.Context, logicalSecretName, key string) (string, bool, error) {
	if r == nil {
		return "", false, nil
	}

	secretName := logicalSecretName

	if len(r.namedInternalSecretNames) > 0 {
		tempName, ok := r.namedInternalSecretNames[logicalSecretName]
		if ok {
			secretName = tempName
		}
	}

	secretKey := types.NamespacedName{
		Namespace: r.secretNamespace,
		Name:      secretName,
	}

	secret := corev1.Secret{}
	err := r.secretClient.Get(ctx, secretKey, &secret)
	if err != nil {
		return "", false, err
	}

	value, ok := secret.Data[key]
	if !ok {
		return "", false, nil
	}

	return string(value), true, nil
}
