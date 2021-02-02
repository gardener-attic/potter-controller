package util

import (
	"context"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetTargetKubeConfig(ctx context.Context, gardenClient client.Client, config *IntegrationTestConfig) string {
	targetClusterSecretKey := types.NamespacedName{
		Namespace: config.GardenNamespace,
		Name:      config.TargetClusterName + ".kubeconfig",
	}

	var kubeConfigSecret corev1.Secret
	err := gardenClient.Get(ctx, targetClusterSecretKey, &kubeConfigSecret)
	if err != nil {
		Write(err, "Unable to read secret with kubeconfig for target cluster "+targetClusterSecretKey.String())
		os.Exit(1)
	}

	targetKubeConfig, ok := kubeConfigSecret.Data["kubeconfig"]
	if !ok {
		Write(err, "Secret contains no kubeconfig for target cluster "+targetClusterSecretKey.String())
		os.Exit(1)
	}

	return string(targetKubeConfig)
}
