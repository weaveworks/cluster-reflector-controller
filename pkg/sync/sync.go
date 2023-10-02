package sync

import (
	"context"
	"fmt"

	"github.com/fluxcd/pkg/apis/meta"
	gitopsv1alpha1 "github.com/weaveworks/cluster-controller/api/v1alpha1"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
	"github.com/weaveworks/weave-gitops/core/logger"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SyncCluster takes a ProviderCluster definition and make sure it exists in the cluster
func SyncCluster(ctx context.Context, k8sClient client.Client, namespace string, cluster *providers.ProviderCluster) (*gitopsv1alpha1.GitopsCluster, *corev1.Secret, error) {
	secretName := fmt.Sprintf("%s-kubeconfig", cluster.Name)

	// Create the secret that will hold the kubeconfig data
	secret, err := CreateOrUpdateGitOpsClusterSecret(ctx, k8sClient, namespace, secretName, cluster.KubeConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create or update GitopsClusterSecret: %w", err)
	}

	// Create or update the GitopsCluster object
	gitopsCluster, err := CreateOrUpdateGitOpsCluster(ctx, k8sClient, namespace, cluster.Name, secretName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create or update GitopsCluster: %w", err)
	}

	return gitopsCluster, secret, nil
}

// CreateOrUpdateGitOpsClusterSecret updates/creates the secret with the kubeconfig data given the secret name and namespace of the secret
func CreateOrUpdateGitOpsClusterSecret(ctx context.Context, k8sClient client.Client, namespace, secretName string, config *clientcmdapi.Config) (*corev1.Secret, error) {
	lgr := log.FromContext(ctx)
	configBytes, err := clientcmd.Write(*config)
	if err != nil {
		return nil, err
	}

	// fetch the secret using the controller-runtime client
	secret := &corev1.Secret{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: namespace}, secret)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		secret = &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"value": configBytes,
			},
		}
		// create the secret
		err = k8sClient.Create(ctx, secret)
		if err != nil {
			return nil, err
		}
		lgr.V(logger.LogLevelDebug).Info("new secret with kubeconfig data created", "secret", secretName)
	}

	secret.Data["value"] = configBytes
	err = k8sClient.Update(ctx, secret)
	if err != nil {
		return nil, err
	}
	lgr.V(logger.LogLevelDebug).Info("secret updated with kubeconfig data successfully")

	return secret, nil
}

// CreateOrUpdateGitOpsCluster creates or updates the GitopsCluster object
func CreateOrUpdateGitOpsCluster(ctx context.Context, k8sClient client.Client, namespace, clusterName, secretName string) (*gitopsv1alpha1.GitopsCluster, error) {
	lgr := log.FromContext(ctx)
	lgr.WithValues("cluster", clusterName, "secret", secretName, "namespace", namespace)

	gitOpsCluster := &gitopsv1alpha1.GitopsCluster{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: namespace}, gitOpsCluster)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get GitopsCluster: %w", err)
		}

		gitOpsCluster = &gitopsv1alpha1.GitopsCluster{
			TypeMeta: metav1.TypeMeta{
				Kind:       "GitopsCluster",
				APIVersion: gitopsv1alpha1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: namespace,
			},
			Spec: gitopsv1alpha1.GitopsClusterSpec{
				SecretRef: &meta.LocalObjectReference{
					Name: secretName,
				},
			},
		}
		err = k8sClient.Create(ctx, gitOpsCluster)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitopsCluster: %w", err)
		}
		lgr.V(logger.LogLevelDebug).Info("GitopsCluster created")
	}

	gitOpsCluster.Spec.SecretRef = &meta.LocalObjectReference{
		Name: secretName,
	}

	err = k8sClient.Update(ctx, gitOpsCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to update GitopsCluster: %w", err)
	}

	lgr.V(logger.LogLevelDebug).Info("GitopsCluster updated")

	return gitOpsCluster, nil
}
