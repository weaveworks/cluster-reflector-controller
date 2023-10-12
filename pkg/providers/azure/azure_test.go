package azure

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	acs "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"

	"github.com/google/go-cmp/cmp"
)

func TestClusterProvider_ListClusters(t *testing.T) {
	stubClient := stubManagedClustersClient{
		[]*acs.ManagedCluster{
			{
				ID:   to.Ptr("/subscriptions/ace37984-3d07-4051-9002-d5a52c0ae14b/resourcegroups/demo-team/providers/Microsoft.ContainerService/managedClusters/cluster-1"),
				Name: to.Ptr("cluster-1"),
			},
		},
	}

	provider := NewAzureProvider("test-subscription")
	provider.ClientFactory = func(s string) (AKSClusterClient, error) {
		return stubClient, nil
	}

	provided, err := provider.ListClusters(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	want := []*providers.ProviderCluster{
		{Name: "cluster-1"},
	}
	if diff := cmp.Diff(want, provided); diff != "" {
		t.Fatalf("failed to list clusters:\n%s", diff)
	}
}

type stubManagedClustersClient struct {
	Clusters []*acs.ManagedCluster
}

func (m stubManagedClustersClient) NewListPager(_ *acs.ManagedClustersClientListOptions) *runtime.Pager[acs.ManagedClustersClientListResponse] {
	return runtime.NewPager(runtime.PagingHandler[acs.ManagedClustersClientListResponse]{
		More: func(_ acs.ManagedClustersClientListResponse) bool {
			return false
		},
		Fetcher: func(_ context.Context, _ *acs.ManagedClustersClientListResponse) (acs.ManagedClustersClientListResponse, error) {
			return acs.ManagedClustersClientListResponse{
				ManagedClusterListResult: acs.ManagedClusterListResult{
					Value: m.Clusters,
				},
			}, nil
		},
	})
}

func (m stubManagedClustersClient) ListClusterAdminCredentials(ctx context.Context, resourceGroupName string, resourceName string, options *acs.ManagedClustersClientListClusterAdminCredentialsOptions) (acs.ManagedClustersClientListClusterAdminCredentialsResponse, error) {
	return acs.ManagedClustersClientListClusterAdminCredentialsResponse{}, nil
}
