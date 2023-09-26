package sync

import (
	"context"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/stretchr/testify/assert"
	gitopsv1alpha1 "github.com/weaveworks/cluster-controller/api/v1alpha1"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSyncCluster(t *testing.T) {
	// fake controller-runtime client
	scheme := runtime.NewScheme()
	err := gitopsv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)
	err = corev1.AddToScheme(scheme)
	assert.NoError(t, err)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// create a kubeconfig
	config := clientcmdapi.NewConfig()
	assert.NoError(t, err)

	// create the cluster provider
	clusterProvider := &providers.ProviderCluster{
		Name:       "test-cluster",
		KubeConfig: config,
	}

	// sync it
	_, _, err = SyncCluster(context.TODO(), k8sClient, "default", clusterProvider)
	assert.NoError(t, err)

	// get the cluster from the client
	clusterRetrieved := &gitopsv1alpha1.GitopsCluster{}
	err = k8sClient.Get(context.TODO(), client.ObjectKey{
		Name:      "test-cluster",
		Namespace: corev1.NamespaceDefault,
	}, clusterRetrieved)
	assert.NoError(t, err)

	expectedCluster := &gitopsv1alpha1.GitopsCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GitopsCluster",
			APIVersion: "gitops.weave.works/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cluster",
			Namespace:       corev1.NamespaceDefault,
			ResourceVersion: "2",
		},
		Spec: gitopsv1alpha1.GitopsClusterSpec{
			SecretRef: &meta.LocalObjectReference{
				Name: "test-cluster-kubeconfig",
			},
		},
	}

	assert.Equal(t, expectedCluster, clusterRetrieved, "Retrieved gitops cluster not equal expected")

	// check the secret was created too
	secretRetrieved := &corev1.Secret{}
	err = k8sClient.Get(context.TODO(), client.ObjectKey{
		Name:      "test-cluster-kubeconfig",
		Namespace: corev1.NamespaceDefault,
	}, secretRetrieved)
	assert.NoError(t, err)

	configBytes, err := clientcmd.Write(*config)
	assert.NoError(t, err)
	expectedSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cluster-kubeconfig",
			Namespace:       corev1.NamespaceDefault,
			ResourceVersion: "2",
		},
		Data: map[string][]byte{
			"value": configBytes,
		},
	}

	assert.Equal(t, expectedSecret, secretRetrieved, "Retrieved secret not equal expected")
}

func TestCreateOrUpdateGitOpsClusterSecret(t *testing.T) {
	// fake controller-runtime client
	k8sClient := fake.NewClientBuilder().Build()

	// create a kubeconfig and secret
	secretName := "spoke-secret"
	config := clientcmdapi.NewConfig()
	configBytes, err := clientcmd.Write(*config)
	assert.NoError(t, err)

	//serialize config
	expectedSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            secretName,
			Namespace:       corev1.NamespaceDefault,
			ResourceVersion: "2",
		},
		Data: map[string][]byte{
			"value": configBytes,
		},
	}

	secretCreated, err := CreateOrUpdateGitOpsClusterSecret(context.TODO(), k8sClient, "default", "spoke-secret", config)
	assert.NoError(t, err)
	assert.Equal(t, expectedSecret, secretCreated, "Secret created not equal expected")

	// get the secret from the client
	secretRetrieved := &corev1.Secret{}
	err = k8sClient.Get(context.TODO(), client.ObjectKey{
		Name:      secretName,
		Namespace: corev1.NamespaceDefault,
	}, secretRetrieved)

	assert.NoError(t, err)
	assert.Equal(t, expectedSecret, secretRetrieved, "Secret retrieved from client not equal expected")
}

func TestCreateOrUpdateGitOpsCluster(t *testing.T) {
	// fake controller-runtime client
	scheme := runtime.NewScheme()
	err := gitopsv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	expectedCluster := &gitopsv1alpha1.GitopsCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GitopsCluster",
			APIVersion: "gitops.weave.works/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cluster",
			Namespace:       corev1.NamespaceDefault,
			ResourceVersion: "2",
		},
		Spec: gitopsv1alpha1.GitopsClusterSpec{
			SecretRef: &meta.LocalObjectReference{
				Name: "test-cluster-kubeconfig",
			},
		},
	}

	gitopsCluster, err := CreateOrUpdateGitOpsCluster(context.TODO(), k8sClient, "default", "test-cluster", "test-cluster-kubeconfig")
	assert.NoError(t, err)
	assert.Equal(t, expectedCluster, gitopsCluster, "Created gitops cluster not equal expected")

	// get the cluster from the client
	clusterRetrieved := &gitopsv1alpha1.GitopsCluster{}
	err = k8sClient.Get(context.TODO(), client.ObjectKey{
		Name:      "test-cluster",
		Namespace: corev1.NamespaceDefault,
	}, clusterRetrieved)

	assert.NoError(t, err)
	assert.Equal(t, expectedCluster, clusterRetrieved, "Retrieved gitops cluster not equal expected")
}
