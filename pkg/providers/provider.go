package providers

import (
	"context"

	"k8s.io/client-go/tools/clientcmd/api"
)

type Provider interface {
	ListClusters(ctx context.Context) ([]*ProviderCluster, error)
}

type ProviderCluster struct {
	Name       string
	KubeConfig *api.Config
}
