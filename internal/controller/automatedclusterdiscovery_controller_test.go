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
		}

		assert.NoError(t, reconciler.SetupWithManager(mgr))

		ctx := context.TODO()
		key := types.NamespacedName{Name: aksCluster.Name, Namespace: aksCluster.Namespace}
		err = mgr.GetClient().Create(ctx, aksCluster)
		assert.NoError(t, err)
		defer deleteClusterDiscoveryAndInventory(t, k8sClient, aksCluster)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		assert.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: time.Minute}, result)

		gitopsCluster := &gitopsv1alpha1.GitopsCluster{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-1", Namespace: aksCluster.Namespace}, gitopsCluster)
		assert.NoError(t, err)
		assert.Equal(t, gitopsv1alpha1.GitopsClusterSpec{
			SecretRef: &meta.LocalObjectReference{Name: "cluster-1-kubeconfig"},
		}, gitopsCluster.Spec)

		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-1-kubeconfig", Namespace: aksCluster.Namespace}, secret)
		assert.NoError(t, err)

		value, err := clientcmd.Write(*testProvider.response[0].KubeConfig)
		assert.NoError(t, err)
		assert.Equal(t, value, secret.Data["value"])

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
		assert.NoError(t, err)

		assertInventoryHasItems(t, aksCluster,
			newSecret(client.ObjectKeyFromObject(secret), nil),
			newGitopsCluster(secret.GetName(), client.ObjectKeyFromObject(gitopsCluster)))

		clusterRef := metav1.OwnerReference{
			Kind:       "AutomatedClusterDiscovery",
			APIVersion: "clusters.weave.works/v1alpha1",
			Name:       aksCluster.Name,
			UID:        aksCluster.UID,
		}
		assertHasOwnerReference(t, gitopsCluster, clusterRef)
		assertHasOwnerReference(t, secret, clusterRef)
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
			"cluster-1-kubeconfig",
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
		}
		assert.NoError(t, reconciler.SetupWithManager(mgr))

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.NoError(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
		assert.NoError(t, err)

		secret := newSecret(types.NamespacedName{Name: "cluster-1-kubeconfig", Namespace: "default"}, []byte(`value`))
		assertInventoryHasItems(t, aksCluster, secret, gitopsCluster)

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
			"cluster-1-kubeconfig",
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
		}
		assert.NoError(t, reconciler.SetupWithManager(mgr))

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.NoError(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(aksCluster), aksCluster)
		assert.NoError(t, err)

		secret := newSecret(types.NamespacedName{Name: "cluster-1-kubeconfig", Namespace: "default"}, nil)
		assertInventoryHasItems(t, aksCluster, secret, gitopsCluster)

		cluster.KubeConfig.Clusters["cluster-1"].Server = "https://cluster-test.example.com/"

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(aksCluster)})
		assert.NoError(t, err)

		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
		assert.NoError(t, err)

		value, err := clientcmd.Write(*testProvider.response[0].KubeConfig)
		assert.NoError(t, err)
		assert.Equal(t, value, secret.Data["value"])
	})
}

type stubProvider struct {
	response []*providers.ProviderCluster
}

func (s *stubProvider) ListClusters(ctx context.Context) ([]*providers.ProviderCluster, error) {
	return s.response, nil
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

	for _, v := range cd.Status.Inventory.Entries {
		u, err := unstructuredFromResourceRef(v)
		if err != nil {
			t.Errorf("failed to convert unstructured from %s", v)
			continue
		}
		if err := client.IgnoreNotFound(cl.Delete(ctx, u)); err != nil {
			t.Errorf("failed to delete %v: %s", u, err)
		}
	}

	if err := cl.Delete(ctx, cd); err != nil {
		t.Fatal(err)
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

func assertHasOwnerReference(t *testing.T, obj metav1.Object, ownerRef metav1.OwnerReference) {
	for _, ref := range obj.GetOwnerReferences() {
		t.Logf("comparing %#v with %#v", ref, ownerRef)
		if isOwnerReferenceEqual(ref, ownerRef) {
			return
		}
	}

	t.Fatalf("%s %s does not have OwnerReference %s", obj.GetResourceVersion(), obj.GetName(), &ownerRef)
}

func isOwnerReferenceEqual(a, b metav1.OwnerReference) bool {
	return (a.APIVersion == b.APIVersion) &&
		(a.Kind == b.Kind) &&
		(a.Name == b.Name) &&
		(a.UID == b.UID)
}
