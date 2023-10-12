package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	acs "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// AKSclusterLister implementations query for AKS clusters.
type AKSClusterClient interface {
	NewListPager(options *acs.ManagedClustersClientListOptions) *runtime.Pager[acs.ManagedClustersClientListResponse]
	ListClusterAdminCredentials(ctx context.Context, resourceGroupName string, resourceName string, options *acs.ManagedClustersClientListClusterAdminCredentialsOptions) (acs.ManagedClustersClientListClusterAdminCredentialsResponse, error)
}

func clientFactory(subscriptionID string) (AKSClusterClient, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain a credential: %v", err)
	}
	client, err := acs.NewManagedClustersClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	return client, nil
}

// AzureProvider queries all AKS clusters for the provided SubscriptionID and
// returns the clusters and kubeconfigs for the clusters.
type AzureProvider struct {
	SubscriptionID string
	ClientFactory  func(string) (AKSClusterClient, error)
}

// NewAzureProvider creates and returns an AzureProvider ready for use.
func NewAzureProvider(subscriptionID string) *AzureProvider {
	return &AzureProvider{
		SubscriptionID: subscriptionID,
		ClientFactory:  clientFactory,
	}
}

func (p *AzureProvider) ListClusters(ctx context.Context) ([]*providers.ProviderCluster, error) {
	clusters := []*providers.ProviderCluster{}
	client, err := p.ClientFactory(p.SubscriptionID)
	if err != nil {
		return nil, err
	}

	pager := client.NewListPager(nil)
	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to advance page: %v", err)
		}
		for _, aksCluster := range nextResult.Value {
			kubeConfig, err := getKubeConfigForCluster(ctx, client, aksCluster)
			if err != nil {
				return nil, fmt.Errorf("failed to get kubeconfig for cluster: %v", err)
			}

			clusters = append(clusters, &providers.ProviderCluster{
				Name:       *aksCluster.Name,
				KubeConfig: kubeConfig,
			})
		}
	}

	return clusters, nil
}

func getKubeConfigForCluster(ctx context.Context, client AKSClusterClient, aksCluster *acs.ManagedCluster) (*clientcmdapi.Config, error) {
	resourceGroup, err := aksClusterResourceGroup(*aksCluster.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster resource group: %v", err)
	}

	credentialsResponse, err := client.ListClusterAdminCredentials(ctx,
		resourceGroup,
		*aksCluster.Name,
		&acs.ManagedClustersClientListClusterAdminCredentialsOptions{ServerFqdn: nil},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get cluster credentials: %v", err)
	}

	var kubeConfig *clientcmdapi.Config
	if credentialsResponse.Kubeconfigs != nil {
		kubeConfigBytes := credentialsResponse.Kubeconfigs[0].Value
		kubeConfig, err = clientcmd.Load(kubeConfigBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig: %v", err)
		}
	}

	return kubeConfig, nil
}

func aksClusterResourceGroup(clusterID string) (string, error) {
	resource, err := azure.ParseResourceID(clusterID)
	if err != nil {
		return "", fmt.Errorf("failed to parse cluster resource group from id: %v", err)
	}
	return resource.ResourceGroup, nil
}
