package azure

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	acs "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/google/go-cmp/cmp"

	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
)

const testSubscriptionID = "ace37984-3d07-4051-9002-d5a52c0ae14b"

var _ providers.Provider = (*AzureProvider)(nil)

func TestAzureProvider_ClusterID(t *testing.T) {
	clusterIDTests := []struct {
		name string
		objs []client.Object
		want string
	}{
		{
			name: "ConfigMap exists",
			objs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "extension-manager-config",
						Namespace: "kube-system",
					},
					Data: map[string]string{
						"AZURE_RESOURCE_GROUP":                 "team-pesto-use1",
						"AZURE_RESOURCE_NAME":                  "pestomarketplacetest",
						"AZURE_SUBSCRIPTION_ID":                testSubscriptionID,
						"AKS_USER_ASSIGNED_IDENTITY_CLIENT_ID": "5b3fe934-b769-453b-9374-c672ab0a0a96",
					},
				},
			},
			want: "/subscriptions/ace37984-3d07-4051-9002-d5a52c0ae14b/resourcegroups/team-pesto-use1/providers/Microsoft.ContainerService/managedClusters/pestomarketplacetest",
		},
		{
			// Ths is ok because it should not match an ID provided by the
			// ListClusters method.
			name: "ConfigMap does not exist",
			objs: []client.Object{},
			want: "",
		},
	}

	for _, tt := range clusterIDTests {
		t.Run(tt.name, func(t *testing.T) {
			fc := fake.NewClientBuilder().WithObjects(tt.objs...).Build()

			provider := NewAzureProvider(testSubscriptionID)

			clusterID, err := provider.ClusterID(context.TODO(), fc)
			if err != nil {
				t.Fatal(err)
			}

			if clusterID != tt.want {
				t.Fatalf("ClusterID() got %s, want %s", clusterID, tt.want)
			}
		})
	}
}

func TestClusterProvider_ListClusters(t *testing.T) {
	stubClient := stubManagedClustersClient{
		[]*acs.ManagedCluster{
			{
				ID:   to.Ptr("/subscriptions/ace37984-3d07-4051-9002-d5a52c0ae14b/resourcegroups/team-pesto-use1/providers/Microsoft.ContainerService/managedClusters/pestomarketplacetest"),
				Name: to.Ptr("cluster-1"),
				Tags: map[string]*string{
					"test-tag": to.Ptr("testing"),
				},
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
		{
			ID:   "/subscriptions/ace37984-3d07-4051-9002-d5a52c0ae14b/resourcegroups/team-pesto-use1/providers/Microsoft.ContainerService/managedClusters/pestomarketplacetest",
			Name: "cluster-1",
			Labels: map[string]string{
				"test-tag": "testing",
			},
		},
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
