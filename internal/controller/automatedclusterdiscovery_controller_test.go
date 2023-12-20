package controller

import (
	"context"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	kubeconfig "k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	gitopsv1alpha1 "github.com/weaveworks/cluster-controller/api/v1alpha1"
	clustersv1alpha1 "github.com/weaveworks/cluster-reflector-controller/api/v1alpha1"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
	capiclusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const testCAData = "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURJVENDQWdtZ0F3SUJBZ0lJWVZjd0NTS1ZRVVV3RFFZSktvWklodmNOQVFFTEJRQXdGVEVUTUJFR0ExVUUKQXhNS2EzVmlaWEp1WlhSbGN6QWVGdzB5TWpBMk1qTXlNRE0zTlRoYUZ3MHlNekEyTWpNeU1ETTRNREJhTURReApGekFWQmdOVkJBb1REbk41YzNSbGJUcHRZWE4wWlhKek1Sa3dGd1lEVlFRREV4QnJkV0psY201bGRHVnpMV0ZrCmJXbHVNSUlCSWpBTkJna3Foa2lHOXcwQkFRRUZBQU9DQVE4QU1JSUJDZ0tDQVFFQTMyZHBtRmlMdVFMSlZYUUYKdS9FamhuendPZmVSK3lvazkvMTJENzlEZ1U1OVVjTUVZQ2R1QzZhODVicUhxRXFiVnU2UUdrSFdmUHE4N0FIVApqMnQvM1kxVVprZ0hEOUtBWDZtdldlWnlBV2tRRUlKaHB2UjRtQ3J1T00zTXdUTHpkZlcrS01xVVBHeUZZM1Y0CjZFTmpCQ3RtdVBSaVZ3ZitZZGVwUkpGRHBLZ3MwOHJoMWUvZ3M5VlJWRlBrakIwVVIraHFMLy9KSmJ1S2NyUlgKUE5nWnE2K2N0ajZVaE9lWlZqVXc3WFNXQXZQNjIwMDZmV1V2K2dJYTVaMTRCUUZCN0Q4amhmWlRHNTE0bE04SwpIOXVsWTZJUktpaXcwcjdwdDZZV091VTdBQ3pWdjFkV3ozVUF0WWluSk15RVVOUFpneHp2VFpWNk1jSVB0QisvCkNCMExEd0lEQVFBQm8xWXdWREFPQmdOVkhROEJBZjhFQkFNQ0JhQXdFd1lEVlIwbEJBd3dDZ1lJS3dZQkJRVUgKQXdJd0RBWURWUjBUQVFIL0JBSXdBREFmQmdOVkhTTUVHREFXZ0JUYXVnaWZneTMyYldGbWh5RjRyZlFRNkp2Ugp0akFOQmdrcWhraUc5dzBCQVFzRkFBT0NBUUVBRlVVV3IzS2tNcTBvaVBZcUE5UnVxNWdrTTEwM2hBV2FiQVBGCld5bjRWUmlkZGh2czgwbzR1bWJIN0xZSDdqSlBwdDRxZmR1VlNwLzdWRHFGakNvUkozQUtxd2EveU9vSDF2ZUIKYkVFQW1YQkM5clZEOUtjbVdrVzhtcC9xbFFqOFFvK1I3WFVCN3JxQTR2anJZVUQrYTg5NWFGb1oxTS9HWXpmTwptenNmaWJ6Y2o3RkZwWCtHOG94ZGkwWnY5eUx2WVFuTmU2aDFhWDgveGgzSmkyYlBjR3Y2aDR3RTFuaDdnV1JaClRHcTZzUFJyenlWSzdBc1Z2bk0wQ0tJTEpJN3k4cFB1S1BmajdBMTh6Uit5RDhvOXA2NmYxS2V0VnVaOUlOL1EKN1FoNUJRSXQwWk0xMi9iZ2ZoZDFxTWNrb2RoazF1eFFQSmVzZll1RzAxQ2dya3ZaZVE9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="

func TestAutomatedClusterDiscoveryReconciler(t *testing.T) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			"testdata/crds"},
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("Failed to start test environment: %v", err)
	}
	defer func() {
		if err := testEnv.Stop(); err != nil {
			t.Fatalf("Failed to stop test environment: %v", err)
		}
	}()

	scheme := runtime.NewScheme()
	assert.NoError(t, clustersv1alpha1.AddToScheme(scheme))
	assert.NoError(t, gitopsv1alpha1.AddToScheme(scheme))
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))
	assert.NoError(t, capiclusterv1.AddToScheme(scheme))

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	assert.NoError(t, err)

	mgr, err := manager.New(cfg, manager.Options{
		Scheme: scheme,
	})
	assert.NoError(t, err)

	t.Run("Reconcile with AKS", func(t *testing.T) {
		aksCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-aks",
				Namespace: "default",
			},
			Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
				Type: "aks",
				AKS: &clustersv1alpha1.AKS{
					SubscriptionID: "subscription-123",
				},
				Interval: metav1.Duration{Duration: time.Minute},
			},
		}

		testProvider := stubProvider{
			response: []*providers.ProviderCluster{
				{
					Name: "cluster-1",
					KubeConfig: &kubeconfig.Config{
						APIVersion: "v1",
						Clusters: map[string]*kubeconfig.Cluster{
							"cluster-1": {
								Server:                   "https://cluster-prod.example.com/",
								CertificateAuthorityData: []uint8(testCAData),
							},
						},
					},
				},
			},
		}

		reconciler := &AutomatedClusterDiscoveryReconciler{
			Client: k8sClient,
			Scheme: scheme,
			AKSProvider: func(providerID string) providers.Provider {
				return &testProvider
			},
			EventRecorder: &mockEventRecorder{},
		}

		assert.NoError(t, reconciler.SetupWithManager(mgr))

		ctx := context.TODO()
		key := types.NamespacedName{Name: aksCluster.Name, Namespace: aksCluster.Namespace}
		err = k8sClient.Create(ctx, aksCluster)
		assert.NoError(t, err)
		defer deleteClusterDiscoveryAndInventory(t, k8sClient, aksCluster)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		assert.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: time.Minute}, result)

		wantLabels := map[string]string{
			"app.kubernetes.io/managed-by":          "cluster-reflector-controller",
			"clusters.weave.works/origin-name":      "test-aks",
			"clusters.weave.works/origin-namespace": "default",
			"clusters.weave.works/origin-type":      "aks",
		}

		gitopsCluster := &gitopsv1alpha1.GitopsCluster{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-1", Namespace: aksCluster.Namespace}, gitopsCluster)
		assert.NoError(t, err)
		assert.Equal(t, gitopsv1alpha1.GitopsClusterSpec{
			SecretRef: &meta.LocalObjectReference{Name: "cluster-1-kubeconfig"},
		}, gitopsCluster.Spec)
		assertHasLabels(t, gitopsCluster, wantLabels)

		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-1-kubeconfig", Namespace: aksCluster.Namespace}, secret)
		assert.NoError(t, err)
		assertHasLabels(t, secret, wantLabels)

		value, err := clientcmd.Write(*testProvider.response[0].KubeConfig)
		assert.NoError(t, err)
		assert.Equal(t, value, secret.Data["value"])

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
		assert.NoError(t, err)

		assertInventoryHasItems(t, aksCluster,
			newSecret(client.ObjectKeyFromObject(secret)),
			newGitopsCluster(client.ObjectKeyFromObject(gitopsCluster)))
		assertAutomatedClusterDiscoveryCondition(t, aksCluster, meta.ReadyCondition, "1 clusters discovered")

		discoveryRef := metav1.OwnerReference{
			Kind:       "AutomatedClusterDiscovery",
			APIVersion: "clusters.weave.works/v1alpha1",
			Name:       aksCluster.Name,
			UID:        aksCluster.UID,
		}
		assertHasOwnerReference(t, gitopsCluster, discoveryRef)

		clusterRef := metav1.OwnerReference{
			Kind:       "GitopsCluster",
			APIVersion: "gitops.weave.works/v1alpha1",
			Name:       gitopsCluster.Name,
			UID:        gitopsCluster.UID,
		}
		assertHasOwnerReference(t, secret, clusterRef)
	})

	t.Run("Reconcile with common labels and annotations", func(t *testing.T) {
		wantAnnotations := map[string]string{
			"test.example.com/annotation": "test",
			"example.com/test":            "annotation",
		}

		aksCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-aks",
				Namespace: "default",
			},
			Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
				Type: "aks",
				AKS: &clustersv1alpha1.AKS{
					SubscriptionID: "subscription-123",
				},
				Interval: metav1.Duration{Duration: time.Minute},
				CommonLabels: map[string]string{
					"example.com/label": "test",
				},
				CommonAnnotations: wantAnnotations,
			},
		}

		testProvider := stubProvider{
			response: []*providers.ProviderCluster{
				{
					Name: "cluster-2",
					KubeConfig: &kubeconfig.Config{
						APIVersion: "v1",
						Clusters: map[string]*kubeconfig.Cluster{
							"cluster-2": {
								Server:                   "https://cluster-prod.example.com/",
								CertificateAuthorityData: []uint8(testCAData),
							},
						},
					},
				},
			},
		}

		reconciler := &AutomatedClusterDiscoveryReconciler{
			Client: k8sClient,
			Scheme: scheme,
			AKSProvider: func(providerID string) providers.Provider {
				return &testProvider
			},
			EventRecorder: &mockEventRecorder{},
		}

		assert.NoError(t, reconciler.SetupWithManager(mgr))

		ctx := context.TODO()
		key := types.NamespacedName{Name: aksCluster.Name, Namespace: aksCluster.Namespace}
		err = k8sClient.Create(ctx, aksCluster)
		assert.NoError(t, err)
		defer deleteClusterDiscoveryAndInventory(t, k8sClient, aksCluster)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		assert.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: time.Minute}, result)

		wantLabels := map[string]string{
			"app.kubernetes.io/managed-by":          "cluster-reflector-controller",
			"clusters.weave.works/origin-name":      "test-aks",
			"clusters.weave.works/origin-namespace": "default",
			"clusters.weave.works/origin-type":      "aks",
			"example.com/label":                     "test",
		}

		gitopsCluster := &gitopsv1alpha1.GitopsCluster{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-2", Namespace: aksCluster.Namespace}, gitopsCluster)
		assert.NoError(t, err)
		assert.Equal(t, gitopsv1alpha1.GitopsClusterSpec{
			SecretRef: &meta.LocalObjectReference{Name: "cluster-2-kubeconfig"},
		}, gitopsCluster.Spec)
		assertHasLabels(t, gitopsCluster, wantLabels)
		assertHasAnnotations(t, gitopsCluster, mergeMaps(wantAnnotations, map[string]string{
			gitopsv1alpha1.GitOpsClusterNoSecretFinalizerAnnotation: "true",
		}))

		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-2-kubeconfig", Namespace: aksCluster.Namespace}, secret)
		assert.NoError(t, err)
		assertHasLabels(t, secret, wantLabels)
		assertHasAnnotations(t, secret, wantAnnotations)
	})

	t.Run("Reconcile with cluster labels applies labels to generated cluster", func(t *testing.T) {
		aksCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-aks",
				Namespace: "default",
			},
			Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
				Type: "aks",
				AKS: &clustersv1alpha1.AKS{
					SubscriptionID: "subscription-123",
				},
				Interval: metav1.Duration{Duration: time.Minute},
			},
		}

		testProvider := stubProvider{
			response: []*providers.ProviderCluster{
				{
					Name: "cluster-with-labels",
					Labels: map[string]string{
						"com.example/testing-tag": "test-value",
					},
					KubeConfig: &kubeconfig.Config{
						APIVersion: "v1",
						Clusters: map[string]*kubeconfig.Cluster{
							"cluster-1": {
								Server:                   "https://cluster-prod.example.com/",
								CertificateAuthorityData: []uint8(testCAData),
							},
						},
					},
				},
			},
		}

		reconciler := &AutomatedClusterDiscoveryReconciler{
			Client: k8sClient,
			Scheme: scheme,
			AKSProvider: func(providerID string) providers.Provider {
				return &testProvider
			},
			EventRecorder: &mockEventRecorder{},
		}

		assert.NoError(t, reconciler.SetupWithManager(mgr))

		ctx := context.TODO()
		key := types.NamespacedName{Name: aksCluster.Name, Namespace: aksCluster.Namespace}
		err = k8sClient.Create(ctx, aksCluster)
		assert.NoError(t, err)
		defer deleteClusterDiscoveryAndInventory(t, k8sClient, aksCluster)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		assert.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: time.Minute}, result)

		wantLabels := map[string]string{
			"app.kubernetes.io/managed-by":          "cluster-reflector-controller",
			"clusters.weave.works/origin-name":      "test-aks",
			"clusters.weave.works/origin-namespace": "default",
			"clusters.weave.works/origin-type":      "aks",
		}

		gitopsCluster := &gitopsv1alpha1.GitopsCluster{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-with-labels", Namespace: aksCluster.Namespace}, gitopsCluster)
		assert.NoError(t, err)
		assert.Equal(t, gitopsv1alpha1.GitopsClusterSpec{
			SecretRef: &meta.LocalObjectReference{Name: "cluster-with-labels-kubeconfig"},
		}, gitopsCluster.Spec)
		assertHasLabels(t, gitopsCluster, mergeMaps(wantLabels, map[string]string{
			"com.example/testing-tag": "test-value",
		}))
		assertHasAnnotations(t, gitopsCluster, map[string]string{
			gitopsv1alpha1.GitOpsClusterNoSecretFinalizerAnnotation: "true",
		})

		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-with-labels-kubeconfig", Namespace: aksCluster.Namespace}, secret)
		assert.NoError(t, err)
		assertHasLabels(t, secret, wantLabels)
		assertHasAnnotations(t, secret, map[string]string{})
	})

	t.Run("Reconcile with cluster labels does not apply labels to cluster when tags disabled", func(t *testing.T) {
		aksCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-aks-disabled-tags",
				Namespace: "default",
			},
			Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
				Type:        "aks",
				DisableTags: true,
				AKS: &clustersv1alpha1.AKS{
					SubscriptionID: "subscription-123",
				},
				Interval: metav1.Duration{Duration: time.Minute},
			},
		}

		testProvider := stubProvider{
			response: []*providers.ProviderCluster{
				{
					Name: "test-cluster",
					Labels: map[string]string{
						"com.example/testing-tag": "test-value",
					},
					KubeConfig: &kubeconfig.Config{
						APIVersion: "v1",
						Clusters: map[string]*kubeconfig.Cluster{
							"cluster-1": {
								Server:                   "https://cluster-prod.example.com/",
								CertificateAuthorityData: []uint8(testCAData),
							},
						},
					},
				},
			},
		}

		reconciler := &AutomatedClusterDiscoveryReconciler{
			Client: k8sClient,
			Scheme: scheme,
			AKSProvider: func(providerID string) providers.Provider {
				return &testProvider
			},
			EventRecorder: &mockEventRecorder{},
		}

		assert.NoError(t, reconciler.SetupWithManager(mgr))

		ctx := context.TODO()
		key := types.NamespacedName{Name: aksCluster.Name, Namespace: aksCluster.Namespace}
		err = k8sClient.Create(ctx, aksCluster)
		assert.NoError(t, err)
		defer deleteClusterDiscoveryAndInventory(t, k8sClient, aksCluster)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		assert.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: time.Minute}, result)

		wantLabels := map[string]string{
			"app.kubernetes.io/managed-by":          "cluster-reflector-controller",
			"clusters.weave.works/origin-name":      "test-aks-disabled-tags",
			"clusters.weave.works/origin-namespace": "default",
			"clusters.weave.works/origin-type":      "aks",
		}

		gitopsCluster := &gitopsv1alpha1.GitopsCluster{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-cluster", Namespace: aksCluster.Namespace}, gitopsCluster)
		assert.NoError(t, err)
		assert.Equal(t, gitopsv1alpha1.GitopsClusterSpec{
			SecretRef: &meta.LocalObjectReference{Name: "test-cluster-kubeconfig"},
		}, gitopsCluster.Spec)
		assertHasLabels(t, gitopsCluster, wantLabels)
		assertHasAnnotations(t, gitopsCluster, map[string]string{
			gitopsv1alpha1.GitOpsClusterNoSecretFinalizerAnnotation: "true",
		})

		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-cluster-kubeconfig", Namespace: aksCluster.Namespace}, secret)
		assert.NoError(t, err)
		assertHasLabels(t, secret, wantLabels)
		assertHasAnnotations(t, secret, map[string]string{})
	})

	t.Run("Reconcile when executing in cluster and cluster matches reflector cluster", func(t *testing.T) {
		aksCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-aks",
				Namespace: "default",
			},
			Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
				Type: "aks",
				AKS: &clustersv1alpha1.AKS{
					SubscriptionID: "subscription-123",
				},
				Interval: metav1.Duration{Duration: time.Minute},
			},
		}
		testClusterID := "/subscriptions/ace37984-aaaa-1234-1234-a1a12c0ae14b/resourcegroups/team-pesto-use1/providers/Microsoft.ContainerService/managedClusters/test-cluster"

		testProvider := stubProvider{
			clusterID: testClusterID,
			response: []*providers.ProviderCluster{
				{
					Name: "test-cluster",
					ID:   testClusterID,
					KubeConfig: &kubeconfig.Config{
						APIVersion: "v1",
						Clusters: map[string]*kubeconfig.Cluster{
							"test-cluster": {
								Server:                   "https://cluster-prod.example.com/",
								CertificateAuthorityData: []uint8(testCAData),
							},
						},
					},
				},
			},
		}

		reconciler := &AutomatedClusterDiscoveryReconciler{
			Client: k8sClient,
			Scheme: scheme,
			AKSProvider: func(providerID string) providers.Provider {
				return &testProvider
			},
			EventRecorder: &mockEventRecorder{},
		}

		assert.NoError(t, reconciler.SetupWithManager(mgr))

		ctx := context.TODO()
		key := types.NamespacedName{Name: aksCluster.Name, Namespace: aksCluster.Namespace}
		err = k8sClient.Create(ctx, aksCluster)
		assert.NoError(t, err)
		defer deleteClusterDiscoveryAndInventory(t, k8sClient, aksCluster)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		assert.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: time.Minute}, result)

		cl := &gitopsv1alpha1.GitopsClusterList{}
		assert.NoError(t, k8sClient.List(ctx, cl, &client.ListOptions{Namespace: corev1.NamespaceDefault}))

		gitopsCluster := &gitopsv1alpha1.GitopsCluster{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-cluster", Namespace: aksCluster.Namespace}, gitopsCluster)
		assert.True(t, apierrors.IsNotFound(err))

		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-cluster-kubeconfig", Namespace: aksCluster.Namespace}, secret)
		assert.True(t, apierrors.IsNotFound(err))
	})

	t.Run("Reconcile when cluster has been removed from AKS", func(t *testing.T) {
		ctx := context.TODO()
		aksCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-aks",
				Namespace: "default",
			},
			Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
				Type: "aks",
				AKS: &clustersv1alpha1.AKS{
					SubscriptionID: "subscription-123",
				},
				Interval: metav1.Duration{Duration: time.Minute},
			},
		}

		err := k8sClient.Create(ctx, aksCluster)
		assert.NoError(t, err)
		defer deleteClusterDiscoveryAndInventory(t, k8sClient, aksCluster)

		gitopsCluster := newGitopsCluster(
			types.NamespacedName{Name: "cluster-1", Namespace: "default"},
		)

		testProvider := stubProvider{
			response: []*providers.ProviderCluster{
				{
					Name: "cluster-1",
					KubeConfig: &kubeconfig.Config{
						APIVersion: "v1",
						Clusters: map[string]*kubeconfig.Cluster{
							"cluster-1": {
								Server:                   "https://cluster-prod.example.com/",
								CertificateAuthorityData: []uint8(testCAData),
							},
						},
					},
				},
			},
		}

		reconciler := &AutomatedClusterDiscoveryReconciler{
			Client: k8sClient,
			Scheme: scheme,
			AKSProvider: func(providerID string) providers.Provider {
				return &testProvider
			},
			EventRecorder: &mockEventRecorder{},
		}
		assert.NoError(t, reconciler.SetupWithManager(mgr))

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.NoError(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
		assert.NoError(t, err)

		secret := newSecret(types.NamespacedName{Name: "cluster-1-kubeconfig", Namespace: "default"})
		assertInventoryHasItems(t, aksCluster, secret, gitopsCluster)
		assertAutomatedClusterDiscoveryCondition(t, aksCluster, meta.ReadyCondition, "1 clusters discovered")

		testProvider.response = []*providers.ProviderCluster{}

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.NoError(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsCluster), gitopsCluster)
		assert.True(t, apierrors.IsNotFound(err))
	})

	t.Run("Reconcile updates Secret value for existing clusters", func(t *testing.T) {
		ctx := context.TODO()
		aksCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-aks",
				Namespace: "default",
			},
			Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
				Type: "aks",
				AKS: &clustersv1alpha1.AKS{
					SubscriptionID: "subscription-123",
				},
				Interval: metav1.Duration{Duration: time.Minute},
			},
		}

		err := k8sClient.Create(ctx, aksCluster)
		assert.NoError(t, err)
		defer deleteClusterDiscoveryAndInventory(t, k8sClient, aksCluster)

		gitopsCluster := newGitopsCluster(
			types.NamespacedName{Name: "cluster-1", Namespace: "default"},
		)

		cluster := &providers.ProviderCluster{
			Name: "cluster-1",
			KubeConfig: &kubeconfig.Config{
				APIVersion: "v1",
				Clusters: map[string]*kubeconfig.Cluster{
					"cluster-1": {
						Server:                   "https://cluster-prod.example.com/",
						CertificateAuthorityData: []uint8(testCAData),
					},
				},
			},
		}

		testProvider := stubProvider{
			response: []*providers.ProviderCluster{
				cluster,
			},
		}

		reconciler := &AutomatedClusterDiscoveryReconciler{
			Client: k8sClient,
			Scheme: scheme,
			AKSProvider: func(providerID string) providers.Provider {
				return &testProvider
			},
			EventRecorder: &mockEventRecorder{},
		}
		assert.NoError(t, reconciler.SetupWithManager(mgr))

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.NoError(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
		assert.NoError(t, err)

		secret := newSecret(types.NamespacedName{Name: "cluster-1-kubeconfig", Namespace: "default"})
		assertInventoryHasItems(t, aksCluster, secret, gitopsCluster)
		assertAutomatedClusterDiscoveryCondition(t, aksCluster, meta.ReadyCondition, "1 clusters discovered")

		cluster.KubeConfig.Clusters["cluster-1"].Server = "https://cluster-test.example.com/"

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.NoError(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
		assert.NoError(t, err)

		value, err := clientcmd.Write(*testProvider.response[0].KubeConfig)
		assert.NoError(t, err)
		assert.Equal(t, value, secret.Data["value"])
	})

	t.Run("Reconcile suspended cluster discovery resource", func(t *testing.T) {
		ctx := context.TODO()
		aksCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-aks",
				Namespace: "default",
			},
			Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
				Type: "aks",
				AKS: &clustersv1alpha1.AKS{
					SubscriptionID: "subscription-123",
				},
				Interval: metav1.Duration{Duration: time.Minute},
				Suspend:  true,
			},
		}

		testProvider := stubProvider{
			response: []*providers.ProviderCluster{
				{
					Name: "cluster-1",
					KubeConfig: &kubeconfig.Config{
						APIVersion: "v1",
						Clusters: map[string]*kubeconfig.Cluster{
							"cluster-1": {
								Server:                   "https://cluster-prod.example.com/",
								CertificateAuthorityData: []uint8(testCAData),
							},
						},
					},
				},
			},
		}

		reconciler := &AutomatedClusterDiscoveryReconciler{
			Client: k8sClient,
			Scheme: scheme,
			AKSProvider: func(providerID string) providers.Provider {
				return &testProvider
			},
			EventRecorder: &mockEventRecorder{},
		}

		assert.NoError(t, reconciler.SetupWithManager(mgr))

		key := types.NamespacedName{Name: aksCluster.Name, Namespace: aksCluster.Namespace}
		err = k8sClient.Create(ctx, aksCluster)
		assert.NoError(t, err)
		defer deleteObject(t, k8sClient, aksCluster)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		assert.NoError(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
		assert.NoError(t, err)
		assert.Equal(t, true, aksCluster.Spec.Suspend)

		assertInventoryHasNoItems(t, aksCluster)
	})

	t.Run("Reconcile restores missing resources", func(t *testing.T) {
		ctx := context.TODO()
		aksCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-aks",
				Namespace: "default",
			},
			Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
				Type: "aks",
				AKS: &clustersv1alpha1.AKS{
					SubscriptionID: "subscription-123",
				},
				Interval: metav1.Duration{Duration: time.Minute},
			},
		}

		err := k8sClient.Create(ctx, aksCluster)
		assert.NoError(t, err)
		defer deleteClusterDiscoveryAndInventory(t, k8sClient, aksCluster)

		cluster := &providers.ProviderCluster{
			Name: "cluster-1",
			KubeConfig: &kubeconfig.Config{
				APIVersion: "v1",
				Clusters: map[string]*kubeconfig.Cluster{
					"cluster-1": {
						Server:                   "https://cluster-prod.example.com/",
						CertificateAuthorityData: []uint8(testCAData),
					},
				},
			},
		}

		testProvider := stubProvider{
			response: []*providers.ProviderCluster{
				cluster,
			},
		}

		reconciler := &AutomatedClusterDiscoveryReconciler{
			Client: k8sClient,
			Scheme: scheme,
			AKSProvider: func(providerID string) providers.Provider {
				return &testProvider
			},
			EventRecorder: &mockEventRecorder{},
		}
		assert.NoError(t, reconciler.SetupWithManager(mgr))

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.NoError(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
		assert.NoError(t, err)

		secret := newSecret(types.NamespacedName{Name: "cluster-1-kubeconfig", Namespace: aksCluster.GetNamespace()})
		gitopsCluster := newGitopsCluster(types.NamespacedName{Name: "cluster-1", Namespace: aksCluster.GetNamespace()})
		assertInventoryHasItems(t, aksCluster, secret, gitopsCluster)
		assertAutomatedClusterDiscoveryCondition(t, aksCluster, meta.ReadyCondition, "1 clusters discovered")

		assert.NoError(t, k8sClient.Delete(ctx, secret))
		assert.NoError(t, k8sClient.Delete(ctx, gitopsCluster))

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.NoError(t, err)
	})

	t.Run("Reconcile failing but still records the status", func(t *testing.T) {
		ctx := context.TODO()
		aksCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-aks",
				Namespace: "default",
			},
			Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
				Type: "aks",
				AKS: &clustersv1alpha1.AKS{
					SubscriptionID: "subscription-123",
				},
				Interval: metav1.Duration{Duration: time.Minute},
			},
		}

		err := k8sClient.Create(ctx, aksCluster)
		assert.NoError(t, err)
		defer deleteClusterDiscoveryAndInventory(t, k8sClient, aksCluster)

		cluster := &providers.ProviderCluster{
			Name: "cluster-1",
			KubeConfig: &kubeconfig.Config{
				APIVersion: "v1",
				Clusters: map[string]*kubeconfig.Cluster{
					"cluster-1": {
						Server:                   "https://cluster-prod.example.com/",
						CertificateAuthorityData: []uint8(testCAData),
					},
				},
			},
		}

		testProvider := stubProvider{
			response: []*providers.ProviderCluster{
				cluster,
			},
			responseErr: assert.AnError,
		}

		reconciler := &AutomatedClusterDiscoveryReconciler{
			Client: k8sClient,
			Scheme: scheme,
			AKSProvider: func(providerID string) providers.Provider {
				return &testProvider
			},
		}
		assert.NoError(t, reconciler.SetupWithManager(mgr))

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.Error(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
		assert.NoError(t, err)

		aksCluster.Annotations = map[string]string{
			meta.ReconcileRequestAnnotation: "testing",
		}
		err = k8sClient.Update(ctx, aksCluster)
		assert.NoError(t, err)

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.Error(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
		assert.NoError(t, err)
		assert.Equal(t, "testing", aksCluster.Annotations[meta.ReconcileRequestAnnotation])
		assert.Equal(t, "testing", aksCluster.Status.LastHandledReconcileAt)
	})

	t.Run("Reconcile publishes events on cluster creation and removal", func(t *testing.T) {
		ctx := context.TODO()
		aksCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-aks",
				Namespace: "default",
			},
			Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
				Type: "aks",
				AKS: &clustersv1alpha1.AKS{
					SubscriptionID: "subscription-123",
				},
				Interval: metav1.Duration{Duration: time.Minute},
			},
		}

		err := k8sClient.Create(ctx, aksCluster)
		assert.NoError(t, err)
		defer deleteClusterDiscoveryAndInventory(t, k8sClient, aksCluster)

		cluster := &providers.ProviderCluster{
			Name: "cluster-1",
			KubeConfig: &kubeconfig.Config{
				APIVersion: "v1",
				Clusters: map[string]*kubeconfig.Cluster{
					"cluster-1": {
						Server:                   "https://cluster-prod.example.com/",
						CertificateAuthorityData: []uint8(testCAData),
					},
				},
			},
		}

		testProvider := stubProvider{
			response: []*providers.ProviderCluster{
				cluster,
			},
		}

		mockEventRecorder := &mockEventRecorder{}

		reconciler := &AutomatedClusterDiscoveryReconciler{
			Client: k8sClient,
			Scheme: scheme,
			AKSProvider: func(providerID string) providers.Provider {
				return &testProvider
			},
			EventRecorder: mockEventRecorder,
		}
		assert.NoError(t, reconciler.SetupWithManager(mgr))

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.NoError(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
		assert.NoError(t, err)

		secret := newSecret(types.NamespacedName{Name: "cluster-1-kubeconfig", Namespace: aksCluster.GetNamespace()})
		gitopsCluster := newGitopsCluster(types.NamespacedName{Name: "cluster-1", Namespace: aksCluster.GetNamespace()})
		assertInventoryHasItems(t, aksCluster, secret, gitopsCluster)

		assert.Equal(t, "Normal", mockEventRecorder.CapturedType)
		assert.Equal(t, "ClusterCreated", mockEventRecorder.CapturedReason)
		assert.Equal(t, "Cluster cluster-1 created", mockEventRecorder.CapturedMessage)

		testProvider.response = []*providers.ProviderCluster{}

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.NoError(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
		assert.NoError(t, err)

		assertInventoryHasNoItems(t, aksCluster)

		assert.Equal(t, "Normal", mockEventRecorder.CapturedType)
		assert.Equal(t, "ClusterRemoved", mockEventRecorder.CapturedReason)
		assert.Equal(t, "Cluster cluster-1 removed", mockEventRecorder.CapturedMessage)

		if err := k8sClient.Delete(ctx, secret); err != nil {
			t.Fatal(err)
		}
		assert.NoError(t, client.IgnoreNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)))

	})

	t.Run("Reconcile with CAPI", func(t *testing.T) {
		capiCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-capi",
				Namespace: "default",
			},
			Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
				Type:     "capi",
				Interval: metav1.Duration{Duration: time.Minute},
			},
		}

		testProvider := stubProvider{
			response: []*providers.ProviderCluster{
				{
					Name: "cluster-1",
					KubeConfig: &kubeconfig.Config{
						APIVersion: "v1",
						Clusters: map[string]*kubeconfig.Cluster{
							"cluster-1": {},
						},
					},
				},
			},
		}

		reconciler := &AutomatedClusterDiscoveryReconciler{
			Client: k8sClient,
			Scheme: scheme,
			CAPIProvider: func(capiclient client.Client, namespace, managementClusterName string) providers.Provider {
				return &testProvider
			},
			EventRecorder: &mockEventRecorder{},
		}

		assert.NoError(t, reconciler.SetupWithManager(mgr))

		ctx := context.TODO()
		key := types.NamespacedName{Name: capiCluster.Name, Namespace: capiCluster.Namespace}
		err = k8sClient.Create(ctx, capiCluster)
		assert.NoError(t, err)
		defer deleteClusterDiscoveryAndInventory(t, k8sClient, capiCluster)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		assert.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: time.Minute}, result)

		wantLabels := map[string]string{
			"app.kubernetes.io/managed-by":          "cluster-reflector-controller",
			"clusters.weave.works/origin-name":      "test-capi",
			"clusters.weave.works/origin-namespace": "default",
			"clusters.weave.works/origin-type":      "capi",
		}

		gitopsCluster := &gitopsv1alpha1.GitopsCluster{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-1", Namespace: capiCluster.Namespace}, gitopsCluster)
		assert.NoError(t, err)
		assert.Equal(t, gitopsv1alpha1.GitopsClusterSpec{
			CAPIClusterRef: &meta.LocalObjectReference{Name: "cluster-1"},
		}, gitopsCluster.Spec)
		assertHasLabels(t, gitopsCluster, wantLabels)

		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-1-kubeconfig", Namespace: capiCluster.Namespace}, secret)
		assert.NoError(t, err)
		assertHasLabels(t, secret, wantLabels)

		value, err := clientcmd.Write(*testProvider.response[0].KubeConfig)
		assert.NoError(t, err)
		assert.Equal(t, value, secret.Data["value"])

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(capiCluster), capiCluster)
		assert.NoError(t, err)

		assertInventoryHasItems(t, capiCluster,
			newSecret(client.ObjectKeyFromObject(secret)),
			newGitopsCluster(client.ObjectKeyFromObject(gitopsCluster)))
		assertAutomatedClusterDiscoveryCondition(t, capiCluster, meta.ReadyCondition, "1 clusters discovered")

		discoveryRef := metav1.OwnerReference{
			Kind:       "AutomatedClusterDiscovery",
			APIVersion: "clusters.weave.works/v1alpha1",
			Name:       capiCluster.Name,
			UID:        capiCluster.UID,
		}
		assertHasOwnerReference(t, gitopsCluster, discoveryRef)

		clusterRef := metav1.OwnerReference{
			Kind:       "GitopsCluster",
			APIVersion: "gitops.weave.works/v1alpha1",
			Name:       gitopsCluster.Name,
			UID:        gitopsCluster.UID,
		}
		assertHasOwnerReference(t, secret, clusterRef)
	})

}

func TestReconcilingWithAnnotationChange(t *testing.T) {
	ctx := context.TODO()
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			"testdata/crds"},
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("Failed to start test environment: %v", err)
	}
	defer func() {
		if err := testEnv.Stop(); err != nil {
			t.Fatalf("Failed to stop test environment: %v", err)
		}
	}()

	scheme := runtime.NewScheme()
	assert.NoError(t, clustersv1alpha1.AddToScheme(scheme))
	assert.NoError(t, gitopsv1alpha1.AddToScheme(scheme))
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	assert.NoError(t, err)

	mgr, err := manager.New(cfg, manager.Options{
		Scheme: scheme,
	})
	assert.NoError(t, err)

	aksCluster := &clustersv1alpha1.AutomatedClusterDiscovery{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-aks",
			Namespace: "default",
		},
		Spec: clustersv1alpha1.AutomatedClusterDiscoverySpec{
			Type: "aks",
			AKS: &clustersv1alpha1.AKS{
				SubscriptionID: "subscription-123",
			},
			Interval: metav1.Duration{Duration: time.Minute},
		},
	}

	err = k8sClient.Create(ctx, aksCluster)
	assert.NoError(t, err)
	defer deleteClusterDiscoveryAndInventory(t, k8sClient, aksCluster)

	reconciler := &AutomatedClusterDiscoveryReconciler{
		Client: k8sClient,
		Scheme: scheme,
		AKSProvider: func(providerID string) providers.Provider {
			return &stubProvider{}
		},
		EventRecorder: &mockEventRecorder{},
	}
	assert.NoError(t, reconciler.SetupWithManager(mgr))

	key := types.NamespacedName{Name: aksCluster.Name, Namespace: aksCluster.Namespace}
	_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
	assert.NoError(t, err)

	err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
	assert.NoError(t, err)

	assert.Equal(t, aksCluster.Status.LastHandledReconcileAt, "")

	// add an annotation
	aksCluster.Annotations = map[string]string{
		meta.ReconcileRequestAnnotation: "testing",
	}
	err = k8sClient.Update(ctx, aksCluster)
	assert.NoError(t, err)

	_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
	assert.NoError(t, err)

	err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
	assert.NoError(t, err)
	assert.Equal(t, "testing", aksCluster.Annotations[meta.ReconcileRequestAnnotation])
	assert.Equal(t, aksCluster.Status.LastHandledReconcileAt, "testing")
}

type mockEventRecorder struct {
	CapturedObj     runtime.Object
	CapturedType    string
	CapturedReason  string
	CapturedMessage string
}

func (m *mockEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	m.CapturedObj = object
	m.CapturedType = eventtype
	m.CapturedReason = reason
	m.CapturedMessage = message
}

type stubProvider struct {
	response    []*providers.ProviderCluster
	clusterID   string
	responseErr error
}

func (s *stubProvider) ListClusters(ctx context.Context) ([]*providers.ProviderCluster, error) {
	return s.response, s.responseErr
}

func (s *stubProvider) ClusterID(ctx context.Context, kubeClient client.Reader) (string, error) {
	return s.clusterID, nil
}

func deleteObject(t *testing.T, cl client.Client, obj client.Object) {
	t.Helper()

	if err := cl.Delete(context.TODO(), obj); err != nil {
		t.Fatal(err)
	}
}

func deleteClusterDiscoveryAndInventory(t *testing.T, cl client.Client, cd *clustersv1alpha1.AutomatedClusterDiscovery) {
	t.Helper()
	ctx := context.TODO()

	assert.NoError(t, cl.Get(ctx, client.ObjectKeyFromObject(cd), cd))

	if cd.Status.Inventory != nil {
		for _, v := range cd.Status.Inventory.Entries {
			u, err := unstructuredFromResourceRef(v)
			if err != nil {
				t.Errorf("failed to convert unstructured from %s", v)
				continue
			}
			if err := client.IgnoreNotFound(cl.Delete(ctx, u)); err != nil {
				t.Errorf("failed to delete %v: %s", u, err)
			}
			assert.NoError(t, client.IgnoreNotFound(cl.Get(ctx, client.ObjectKeyFromObject(u), u)))
		}
	}

	if err := cl.Delete(ctx, cd); err != nil {
		t.Fatal(err)
	}
	assert.NoError(t, client.IgnoreNotFound(cl.Get(ctx, client.ObjectKeyFromObject(cd), cd)))
}

func assertAutomatedClusterDiscoveryCondition(t *testing.T, acd *clustersv1alpha1.AutomatedClusterDiscovery, condType, msg string) {
	t.Helper()

	cond := apimeta.FindStatusCondition(acd.Status.Conditions, condType)
	if cond == nil {
		t.Fatalf("failed to find matching status condition for type %s in %#v", condType, acd.Status.Conditions)
	}
	if cond.Message != msg {
		t.Fatalf("got %s, want %s", cond.Message, msg)
	}
}

func assertInventoryHasItems(t *testing.T, acd *clustersv1alpha1.AutomatedClusterDiscovery, objs ...runtime.Object) {
	t.Helper()

	entries := []clustersv1alpha1.ResourceRef{}
	for _, obj := range objs {
		ref, err := clustersv1alpha1.ResourceRefFromObject(obj)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, ref)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	want := &clustersv1alpha1.ResourceInventory{Entries: entries}
	assert.Equal(t, want, acd.Status.Inventory)
}

func assertInventoryHasNoItems(t *testing.T, acd *clustersv1alpha1.AutomatedClusterDiscovery) {
	t.Helper()

	if acd.Status.Inventory == nil {
		return
	}

	if l := len(acd.Status.Inventory.Entries); l != 0 {
		t.Errorf("expected inventory to have 0 items, got %v", l)
	}
}

func assertHasOwnerReference(t *testing.T, obj metav1.Object, ownerRef metav1.OwnerReference) {
	t.Helper()

	for _, ref := range obj.GetOwnerReferences() {
		t.Logf("comparing %#v with %#v", ref, ownerRef)
		if isOwnerReferenceEqual(ref, ownerRef) {
			return
		}
	}

	t.Fatalf("%s %s does not have OwnerReference %s", obj.GetResourceVersion(), obj.GetName(), &ownerRef)
}

func assertHasLabels(t *testing.T, o client.Object, want map[string]string) {
	t.Helper()
	labels := o.GetLabels()

	assert.Equal(t, want, labels)
}

func assertHasAnnotations(t *testing.T, o client.Object, want map[string]string) {
	t.Helper()
	annotations := o.GetAnnotations()
	for k, v := range want {
		kv, ok := annotations[k]
		if !ok {
			t.Errorf("%s %s/%s is missing annotation %q with value %q", o.GetObjectKind().GroupVersionKind().Kind, o.GetNamespace(), o.GetName(), k, v)
			continue
		}
		assert.Equal(t, v, kv)
	}
}

func isOwnerReferenceEqual(a, b metav1.OwnerReference) bool {
	return (a.APIVersion == b.APIVersion) &&
		(a.Kind == b.Kind) &&
		(a.Name == b.Name) &&
		(a.UID == b.UID)
}
