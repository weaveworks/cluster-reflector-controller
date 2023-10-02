package main

import (
	"fmt"

	gitopsv1alpha1 "github.com/weaveworks/cluster-controller/api/v1alpha1"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers/azure"
	"github.com/weaveworks/cluster-reflector-controller/pkg/sync"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	"github.com/spf13/cobra"
)

type GitopsClusterOutput struct {
	GitopsCluster *gitopsv1alpha1.GitopsCluster
	Secret        *corev1.Secret
}

func main() {
	var azureSubscriptionID string
	var namespace string
	var export bool

	var reflectCmd = &cobra.Command{
		Use:   "reflect",
		Short: "Reflect AKS clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			azureProvider := azure.NewAzureProvider(azureSubscriptionID)

			clusters, err := azureProvider.ListClusters(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list clusters: %w", err)
			}

			var k8sClient client.Client

			if !export {
				k8sClient, err = CreateClient()
				if err != nil {
					return fmt.Errorf("failed to create client: %w", err)
				}
			} else {
				k8sClient, err = NewFakeClient()
				if err != nil {
					return fmt.Errorf("failed to create fake client: %w", err)
				}
			}

			exports := []runtime.Object{}
			for _, cluster := range clusters {
				gc, gcs, err := sync.SyncCluster(cmd.Context(), k8sClient, namespace, cluster)
				if err != nil {
					return fmt.Errorf("failed to sync cluster: %w", err)
				}
				exports = append(exports, gc, gcs)
			}

			if export {
				for _, obj := range exports {
					clusterBytes, err := yaml.Marshal(obj)
					if err != nil {
						return fmt.Errorf("failed to marshal GitopsCluster: %w", err)
					}
					fmt.Println("---")
					fmt.Println(string(clusterBytes))
				}

				// print a warning to stderr that you should not save the secret into a file
				// without encypting it first
				fmt.Fprint(cmd.ErrOrStderr(), "\n!!! WARNING !!!\n")
				fmt.Fprint(cmd.ErrOrStderr(), "The secret is not encrypted. Do not save this to a file without encrypting it first.\n\n")
			}

			return nil
		},
	}

	reflectCmd.Flags().StringVar(&azureSubscriptionID, "azure-subscription-id", "", "Azure Subscription ID")
	reflectCmd.Flags().StringVar(&namespace, "namespace", "default", "Namespace to create the GitopsCluster in")
	reflectCmd.Flags().BoolVar(&export, "export", false, "Export resources to stdout")

	var rootCmd = &cobra.Command{Use: "cluster-reflector-cli"}
	rootCmd.AddCommand(reflectCmd)

	rootCmd.Execute()
}

func CreateClient() (client.Client, error) {
	// Initialize the Kubernetes client
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	scheme := runtime.NewScheme()
	err = gitopsv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to add GitopsCluster to scheme: %w", err)
	}

	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to add corev1 to scheme: %w", err)
	}

	// Initialize the controller runtime client
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return k8sClient, nil

}

func NewFakeClient() (client.Client, error) {
	// fake controller-runtime client
	scheme := runtime.NewScheme()
	err := gitopsv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to add GitopsCluster to scheme: %w", err)
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to add corev1 to scheme: %w", err)
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	return k8sClient, nil
}
