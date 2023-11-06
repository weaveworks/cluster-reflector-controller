package azure

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	acs "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AKSclusterLister implementations query for AKS clusters.
type AKSClusterClient interface {
	NewListPager(options *acs.ManagedClustersClientListOptions) *runtime.Pager[acs.ManagedClustersClientListResponse]
	ListClusterAdminCredentials(ctx context.Context, resourceGroupName string, resourceName string, options *acs.ManagedClustersClientListClusterAdminCredentialsOptions) (acs.ManagedClustersClientListClusterAdminCredentialsResponse, error)
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
	client, err := p.ClientFactory(p.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	pager := client.NewListPager(nil)
	clusters := []*providers.ProviderCluster{}
	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to advance page: %v", err)
		}
		for _, aksCluster := range nextResult.Value {
			kubeConfig, err := getKubeconfigForCluster(ctx, client, aksCluster)
			if err != nil {
				return nil, fmt.Errorf("failed to get kubeconfig for cluster: %v", err)
			}

			clusters = append(clusters, &providers.ProviderCluster{
				Name:       *aksCluster.Name,
				ID:         *aksCluster.ID,
				KubeConfig: kubeConfig,
				Labels:     tagsToLabels(aksCluster.Tags),
			})
		}
	}

	return clusters, nil
}

func (p *AzureProvider) ClusterID(ctx context.Context, kubeClient client.Reader) (string, error) {
	configMap := &corev1.ConfigMap{}
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "extension-manager-config", Namespace: "kube-system"}, configMap); err != nil {
		return "", client.IgnoreNotFound(err)
	}

	keys := keysFromConfigMap(configMap,
		"AZURE_RESOURCE_GROUP", "AZURE_RESOURCE_NAME",
		"AZURE_SUBSCRIPTION_ID")

	id := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s/providers/Microsoft.ContainerService/managedClusters/%s",
		keys["AZURE_SUBSCRIPTION_ID"], keys["AZURE_RESOURCE_GROUP"],
		keys["AZURE_RESOURCE_NAME"])

	return id, nil
}

func keysFromConfigMap(configMap *corev1.ConfigMap, keys ...string) map[string]string {
	values := map[string]string{}
	for _, k := range keys {
		v, ok := configMap.Data[k]
		if ok {
			values[k] = v
		}
	}

	return values
}

func getKubeconfigForCluster(ctx context.Context, client AKSClusterClient, aksCluster *acs.ManagedCluster) (*clientcmdapi.Config, error) {
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

// this is the default client factory which just creates a set of
// AzureCredentials and creates a client from it.
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

func tagsToLabels(src map[string]*string) map[string]string {
	labels := map[string]string{}

	for k, v := range src {
		labels[k] = *v
	}

	return labels
}
