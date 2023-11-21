package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ providers.Provider = (*AWSProvider)(nil)

func TestAWSProviderListClusters(t *testing.T) {
	provider := AWSProvider{
		ClientFactory: func(region string) (*awsAPIs, error) {
			return &awsAPIs{eksAPI: &mockEKSAPI{}, stsAPI: &mockSTSAPI{}}, nil
		},
	}

	clusters, err := provider.ListClusters(context.Background())
	assert.NoError(t, err)
	assert.Len(t, clusters, 2)
	expected := []*providers.ProviderCluster{
		{
			Name: "cluster-1",
			ID:   "cluster-1",
			KubeConfig: &clientcmdapi.Config{
				Extensions: map[string]runtime.Object{},
				Preferences: clientcmdapi.Preferences{
					Extensions: map[string]runtime.Object{},
				},
				CurrentContext: "cluster-1",
				Clusters: map[string]*clientcmdapi.Cluster{
					"cluster-1": {
						Server:                   "https://cluster-1-Id.us-west-2.eks.amazonaws.com",
						CertificateAuthorityData: []byte("certificate-data"),
					},
				},
				AuthInfos: map[string]*clientcmdapi.AuthInfo{
					"cluster-1": {
						Token: "k8s-aws-v1.",
					},
				},
				Contexts: map[string]*clientcmdapi.Context{
					"cluster-1": {
						Cluster:  "cluster-1",
						AuthInfo: "cluster-1",
					},
				},
			},
		},
		{
			Name: "cluster-2",
			ID:   "cluster-2",
			KubeConfig: &clientcmdapi.Config{
				Extensions: map[string]runtime.Object{},
				Preferences: clientcmdapi.Preferences{
					Extensions: map[string]runtime.Object{},
				},
				Clusters: map[string]*clientcmdapi.Cluster{
					"cluster-2": {
						Server:                   "https://cluster-2-Id.us-west-2.eks.amazonaws.com",
						CertificateAuthorityData: []byte("certificate-data"),
					},
				},
				AuthInfos: map[string]*clientcmdapi.AuthInfo{
					"cluster-2": {
						Token: "k8s-aws-v1.",
					},
				},
				CurrentContext: "cluster-2",
				Contexts: map[string]*clientcmdapi.Context{
					"cluster-2": {
						Cluster:  "cluster-2",
						AuthInfo: "cluster-2",
					},
				},
			},
		},
	}

	assert.Equal(t, expected, clusters)

}

func TestAWSProvider_ClusterID(t *testing.T) {
	clusterIDTests := []struct {
		name string
		objs []client.Object
		want string
	}{
		{
			name: "Label exists",
			objs: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ip-192-168-111-24.eu-north-1.compute.internal",
						Labels: map[string]string{
							"alpha.eksctl.io/cluster-name": "cluster-1",
						},
					},
				},
			},
			want: "cluster-1",
		},
		{
			name: "missing label",
			objs: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ip-192-168-111-24.eu-north-1.compute.internal",
					},
				},
			},
			want: "",
		},
	}

	for _, tt := range clusterIDTests {
		t.Run(tt.name, func(t *testing.T) {
			fc := fake.NewClientBuilder().WithObjects(tt.objs...).Build()

			provider := AWSProvider{
				ClientFactory: func(region string) (*awsAPIs, error) {
					return &awsAPIs{eksAPI: &mockEKSAPI{}, stsAPI: &mockSTSAPI{}}, nil
				},
			}

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

// mocks and that

type mockEKSAPI struct{}

func (m *mockEKSAPI) ListClusters(input *eks.ListClustersInput) (*eks.ListClustersOutput, error) {
	return &eks.ListClustersOutput{
		Clusters: aws.StringSlice([]string{"cluster-1", "cluster-2"}),
	}, nil
}

func describeClusterOutput(name string) *eks.DescribeClusterOutput {
	// base64 encoded certificate data
	certData := base64.StdEncoding.EncodeToString([]byte("certificate-data"))
	return &eks.DescribeClusterOutput{
		Cluster: &eks.Cluster{
			Name:     aws.String(name),
			Id:       aws.String(fmt.Sprintf("%s-Id", name)),
			Endpoint: aws.String(fmt.Sprintf("https://%s-Id.us-west-2.eks.amazonaws.com", name)),
			CertificateAuthority: &eks.Certificate{
				Data: aws.String(certData),
			},
		},
	}
}

func (m *mockEKSAPI) DescribeCluster(input *eks.DescribeClusterInput) (*eks.DescribeClusterOutput, error) {
	if *input.Name == "cluster-1" {
		return describeClusterOutput("cluster-1"), nil
	} else if *input.Name == "cluster-2" {
		return describeClusterOutput("cluster-2"), nil
	}

	return nil, fmt.Errorf("cluster not found: %s", *input.Name)
}

type mockSTSAPI struct{}

func (m *mockSTSAPI) GetCallerIdentityRequest(input *sts.GetCallerIdentityInput) (*request.Request, *sts.GetCallerIdentityOutput) {
	return &request.Request{
		HTTPRequest: &http.Request{
			Header: http.Header{},
			URL:    &url.URL{},
		},
		Handlers:  request.Handlers{},
		Operation: &request.Operation{},
	}, nil
}
