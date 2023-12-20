package capi

import (
	"context"

	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
	capiclusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CAPIProvider struct {
	Kubeclient            client.Client
	Namespace             string
	ManagementClusterName string
}

var _ providers.Provider = (*CAPIProvider)(nil)

// NewCAPIProvider creates and returns a CAPIProvider ready for use
func NewCAPIProvider(client client.Client, namespace, managementClusterName string) *CAPIProvider {
	provider := &CAPIProvider{
		Kubeclient:            client,
		Namespace:             namespace,
		ManagementClusterName: managementClusterName,
	}
	return provider
}

func (p *CAPIProvider) ListClusters(ctx context.Context) ([]*providers.ProviderCluster, error) {
	kubeClient := p.Kubeclient
	capiClusters := &capiclusterv1.ClusterList{}
	err := kubeClient.List(ctx, capiClusters, &client.ListOptions{Namespace: p.Namespace})
	if err != nil {
		return nil, err
	}

	clusters := []*providers.ProviderCluster{}

	for _, capiCluster := range capiClusters.Items {
		var clusterID string
		if capiCluster.Name == p.ManagementClusterName {
			clusterID = capiCluster.Name + "/" + capiCluster.Namespace
		} else {
			clusterID = string(capiCluster.GetObjectMeta().GetUID())
		}
		clusters = append(clusters, &providers.ProviderCluster{
			Name:       capiCluster.Name,
			ID:         clusterID,
			KubeConfig: nil,
			Labels:     capiCluster.Labels,
		})
	}

	return clusters, nil
}

// ProviderCluster has an ID to identify the cluster, but capi cluster doesn't have a Cluster ID
// therefore wont't match in the case of CAPI
func (p *CAPIProvider) ClusterID(ctx context.Context, kubeClient client.Reader) (string, error) {
	return p.ManagementClusterName, nil
	// return "", nil
}
