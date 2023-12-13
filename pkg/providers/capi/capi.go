package capi

import (
	"context"

	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
	capiclusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CAPIProvider struct {
	Kubeclient client.Client
}

var _ providers.Provider = (*CAPIProvider)(nil)

func NewCAPIProvider(client client.Client) *CAPIProvider {
	provider := &CAPIProvider{
		Kubeclient: client,
	}
	return provider
}

func (p *CAPIProvider) ListClusters(ctx context.Context) ([]*providers.ProviderCluster, error) {
	kubeClient := p.Kubeclient
	capiClusters := &capiclusterv1.ClusterList{}
	err := kubeClient.List(ctx, capiClusters, &client.ListOptions{})
	if err != nil {
		return nil, err
	}

	clusters := []*providers.ProviderCluster{}

	for _, capiCluster := range capiClusters.Items {
		clusters = append(clusters, &providers.ProviderCluster{
			Name:       capiCluster.Name,
			ID:         string(capiCluster.GetObjectMeta().GetUID()),
			KubeConfig: nil,
		})
		// TODO: kubeconfig is in a secret in the namespace of the cluster called clustername-kubeconfig
	}

	return clusters, nil
}

func (p *CAPIProvider) ClusterID(ctx context.Context, kubeClient client.Reader) (string, error) {
	return "", nil
}
