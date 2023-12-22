package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
	corev1 "k8s.io/api/core/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const nodeClusterNameLabel = "alpha.eksctl.io/cluster-name"

// eksAPI is the parts of the EKS API we're interested in.
// Used for listing clusters and getting their cert info.
type eksAPI interface {
	ListClusters(input *eks.ListClustersInput) (*eks.ListClustersOutput, error)
	DescribeCluster(input *eks.DescribeClusterInput) (*eks.DescribeClusterOutput, error)
}

// stsAPI is the parts of the STS API we're interested in.
// Used for generating tokens
type stsAPI interface {
	GetCallerIdentityRequest(input *sts.GetCallerIdentityInput) (req *request.Request, output *sts.GetCallerIdentityOutput)
}

// awsAPIs is the parts of the AWS API we're interested in.
type awsAPIs struct {
	eksAPI
	stsAPI
}

// AWSProvider queries all EKS clusters for the provided AWS region and
// returns the clusters and kubeconfigs for the clusters.
type AWSProvider struct {
	Region        string
	ClientFactory func(string) (*awsAPIs, error)
}

// NewAWSProvider creates and returns an AWSProvider ready for use.
func NewAWSProvider(region string) *AWSProvider {
	return &AWSProvider{
		Region:        region,
		ClientFactory: clientFactory,
	}
}

// ListClusters returns a list of clusters and kubeconfigs for the clusters.
func (p *AWSProvider) ListClusters(ctx context.Context) ([]*providers.ProviderCluster, error) {
	client, err := p.ClientFactory(p.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	clusters := []*providers.ProviderCluster{}
	input := &eks.ListClustersInput{}
	output, err := client.ListClusters(input)
	if err != nil {
		return nil, fmt.Errorf("failed to list EKS clusters: %v", err)
	}

	for _, clusterName := range output.Clusters {
		kubeConfig, err := getKubeconfigForCluster(ctx, client, *clusterName)
		if err != nil {
			return nil, fmt.Errorf("failed to get kubeconfig for cluster %s: %v", *clusterName, err)
		}

		clusters = append(clusters, &providers.ProviderCluster{
			Name:       *clusterName,
			ID:         *clusterName,
			KubeConfig: kubeConfig,
		})
	}

	return clusters, nil
}

// ClusterID returns the ID of the cluster with the provided name.
func (p *AWSProvider) ClusterID(ctx context.Context, kubeClient client.Reader) (string, error) {
	nodes := &corev1.NodeList{}
	if err := kubeClient.List(ctx, nodes); err != nil {
		return "", fmt.Errorf("failed to list nodes: %v", err)
	}

	if len(nodes.Items) == 0 {
		return "", fmt.Errorf("no nodes found")
	}

	node := nodes.Items[0]
	if node.Labels != nil {
		if clusterID, ok := node.Labels[nodeClusterNameLabel]; ok {
			return clusterID, nil
		}
	}

	return "", nil
}

func getKubeconfigForCluster(ctx context.Context, client *awsAPIs, clusterName string) (*clientcmdapi.Config, error) {
	describeInput := &eks.DescribeClusterInput{
		Name: awssdk.String(clusterName),
	}

	describeOutput, err := client.eksAPI.DescribeCluster(describeInput)
	if err != nil {
		return nil, fmt.Errorf("failed to describe EKS cluster %s: %v", clusterName, err)
	}

	kubeConfig, err := generateKubeconfig(describeOutput.Cluster, client.stsAPI, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to generate kubeconfig for cluster %s: %v", clusterName, err)
	}

	return kubeConfig, nil
}

func generateKubeconfig(cluster *eks.Cluster, stsClient stsAPI, clusterName string) (*clientcmdapi.Config, error) {
	kubeconfig := clientcmdapi.NewConfig()

	certData, err := base64.StdEncoding.DecodeString(*cluster.CertificateAuthority.Data)
	if err != nil {
		return nil, fmt.Errorf("decoding cluster CA cert: %w", err)
	}

	kubeconfig.Clusters[clusterName] = &clientcmdapi.Cluster{
		Server:                   *cluster.Endpoint,
		CertificateAuthorityData: certData,
	}

	kubeconfig.Contexts[clusterName] = &clientcmdapi.Context{
		Cluster:  clusterName,
		AuthInfo: clusterName,
	}

	kubeconfig.CurrentContext = clusterName

	token, err := generateToken(stsClient, clusterName)
	if err != nil {
		return nil, fmt.Errorf("generating presigned token: %w", err)
	}

	kubeconfig.AuthInfos[clusterName] = &clientcmdapi.AuthInfo{
		Token: token,
	}

	return kubeconfig, nil
}

const (
	tokenPrefix       = "k8s-aws-v1."
	tokenAgeMins      = 15
	clusterNameHeader = "x-k8s-aws-id"
)

func generateToken(stsClient stsAPI, clusterName string) (string, error) {
	req, _ := stsClient.GetCallerIdentityRequest(&sts.GetCallerIdentityInput{})
	req.HTTPRequest.Header.Add(clusterNameHeader, clusterName)

	presignedURL, err := req.Presign(tokenAgeMins * time.Minute)
	if err != nil {
		return "", fmt.Errorf("presigning AWS get caller identity: %w", err)
	}

	encodedURL := base64.RawURLEncoding.EncodeToString([]byte(presignedURL))
	return fmt.Sprintf("%s%s", tokenPrefix, encodedURL), nil
}

// this is the default client factory which just creates a set of
// aws apis
func clientFactory(region string) (*awsAPIs, error) {
	// Don't specify any credentials here, let the SDK take care of it.
	awsConfig := &awssdk.Config{
		Region: awssdk.String(region),
	}

	session, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}

	return &awsAPIs{
		eksAPI: eks.New(session),
		stsAPI: sts.New(session),
	}, nil
}
