package providers

import (
	"context"

	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Provider implementations query the API for cluster services e.g. EKS and AKS.
type Provider interface {
	// ListClusters queries the cluster service for a list of clusters.
	ListClusters(ctx context.Context) ([]*ProviderCluster, error)

	// ClusterID implementations are responsible for generating a unique ID from
	// a cluster.
	// How this Unique ID is calculated varies across providers.
	// But it must be able to be matched to the UniqueID on a ProviderCluster.
	ClusterID(ctx context.Context, kubeClient client.Reader) (string, error)
}

// ProviderCluster is a representation of the cluster from the cluster service
// that can be used to create a GitopsCluster.
type ProviderCluster struct {
	// This is the name of the cluster as provided by the cluster service.
	Name string
	// This is the unique ID for this cluster.
	UniqueID string

	// This is the KubeConfig from the cluster service used to connect to it.
	// This is likely to be a high privilege token.
	// The token MUST NOT require the execution of a binary (ExecConfig).
	KubeConfig *api.Config
}
