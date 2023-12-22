package capi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	capiclusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clustersv1alpha1 "github.com/weaveworks/cluster-reflector-controller/api/v1alpha1"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
)

func TestClusterProvider_ListClusters(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, capiclusterv1.AddToScheme(scheme))

	// create clusters to list (capi)
	clusters := []client.Object{
		&capiclusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-1",
				Namespace: "default",
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(clusters...).Build()
	provider := NewCAPIProvider(client, "default", clustersv1alpha1.Cluster{Name: "management-cluster"})

	provided, err := provider.ListClusters(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	expected := []*providers.ProviderCluster{
		{
			Name:       "cluster-1",
			ID:         "cluster-1",
			KubeConfig: nil,
		},
	}

	assert.Equal(t, expected, provided, "expected clusters to be equal")

}
