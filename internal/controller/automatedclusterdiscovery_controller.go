/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cli-utils/pkg/object"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/fluxcd/pkg/apis/meta"
	gitopsv1alpha1 "github.com/weaveworks/cluster-controller/api/v1alpha1"
	clustersv1alpha1 "github.com/weaveworks/cluster-reflector-controller/api/v1alpha1"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
	"k8s.io/client-go/tools/clientcmd/api"
)

const k8sManagedByLabel = "app.kubernetes.io/managed-by"

// AutomatedClusterDiscoveryReconciler reconciles a AutomatedClusterDiscovery object
type AutomatedClusterDiscoveryReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	AKSProvider func(string) providers.Provider
}

//+kubebuilder:rbac:groups=clusters.weave.works,resources=automatedclusterdiscoveries,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=clusters.weave.works,resources=automatedclusterdiscoveries/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=clusters.weave.works,resources=automatedclusterdiscoveries/finalizers,verbs=update
//+kubebuilder:rbac:groups=gitops.weave.works,resources=gitopsclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AutomatedClusterDiscoveryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	clusterDiscovery := &clustersv1alpha1.AutomatedClusterDiscovery{}
	if err := r.Get(ctx, req.NamespacedName, clusterDiscovery); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Skip reconciliation if the AutomatedClusterDiscovery is suspended.
	if clusterDiscovery.Spec.Suspend {
		logger.Info("Reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling cluster reflector",
		"type", clusterDiscovery.Spec.Type,
		"name", clusterDiscovery.Spec.Name,
	)

	if clusterDiscovery.Spec.Type == "aks" {
		logger.Info("Reconciling AKS cluster reflector",
			"name", clusterDiscovery.Spec.Name,
		)

		azureProvider := r.AKSProvider(clusterDiscovery.Spec.AKS.SubscriptionID)

		clusters, err := azureProvider.ListClusters(ctx)
		if err != nil {
			logger.Error(err, "Failed to list AKS clusters")
			return ctrl.Result{}, err
		}

		inventoryRefs, err := r.reconcileClusters(ctx, clusters, clusterDiscovery)
		if err != nil {
			return ctrl.Result{}, err
		}

		sort.Slice(inventoryRefs, func(i, j int) bool {
			return inventoryRefs[i].ID < inventoryRefs[j].ID
		})

		clusterDiscovery.Status.Inventory = &clustersv1alpha1.ResourceInventory{Entries: inventoryRefs}

		if err = r.patchStatus(ctx, req, clusterDiscovery.Status); err != nil {
			return ctrl.Result{}, err
		}
	}

	interval := clusterDiscovery.Spec.Interval
	if interval == (metav1.Duration{}) {
		interval = metav1.Duration{Duration: time.Minute * 5}
	}

	return ctrl.Result{RequeueAfter: interval.Duration}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutomatedClusterDiscoveryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clustersv1alpha1.AutomatedClusterDiscovery{}).
		Complete(r)
}

func (r *AutomatedClusterDiscoveryReconciler) reconcileClusters(ctx context.Context, clusters []*providers.ProviderCluster, cd *clustersv1alpha1.AutomatedClusterDiscovery) ([]clustersv1alpha1.ResourceRef, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling clusters", "count", len(clusters))

	clusterMapping := clustersToMapping(clusters)

	isExistingRef := func(ref clustersv1alpha1.ResourceRef, inventory *clustersv1alpha1.ResourceInventory) bool {
		if inventory == nil {
			return false
		}

		for _, v := range inventory.Entries {
			if v == ref {
				return true
			}
		}

		return false
	}

	existingClusters := []clustersv1alpha1.ResourceRef{}
	inventoryResources := []clustersv1alpha1.ResourceRef{}

	for _, cluster := range clusters {
		secretName := fmt.Sprintf("%s-kubeconfig", cluster.Name)

		gitopsCluster := newGitopsCluster(secretName, types.NamespacedName{
			Name:      cluster.Name,
			Namespace: cd.Namespace,
		})

		clusterRef, err := clustersv1alpha1.ResourceRefFromObject(gitopsCluster)
		if err != nil {
			return inventoryResources, err
		}

		if isExistingRef(clusterRef, cd.Status.Inventory) {
			existingClusters = append(existingClusters, clusterRef)
			continue
		}

		logger.Info("creating gitops cluster", "name", gitopsCluster.GetName())
		if err := controllerutil.SetOwnerReference(cd, gitopsCluster, r.Scheme); err != nil {
			return inventoryResources, fmt.Errorf("failed to set ownership on created GitopsCluster: %w", err)
		}
		gitopsCluster.SetLabels(labelsForResource(*cd))
		if err := r.Client.Create(ctx, gitopsCluster); err != nil {
			return inventoryResources, err
		}

		inventoryResources = append(inventoryResources, clusterRef)

		secret, err := newKubeConfigSecret(types.NamespacedName{
			Name:      secretName,
			Namespace: cd.Namespace,
		}, cluster.KubeConfig)
		if err != nil {
			return inventoryResources, err
		}

		secretRef, err := clustersv1alpha1.ResourceRefFromObject(secret)
		if err != nil {
			return inventoryResources, err
		}

		logger.Info("creating secret", "name", secret.GetName())
		if err := controllerutil.SetOwnerReference(cd, secret, r.Scheme); err != nil {
			return inventoryResources, fmt.Errorf("failed to set ownership on created Secret: %w", err)
		}
		secret.SetLabels(labelsForResource(*cd))

		if err := r.Client.Create(ctx, secret); err != nil {
			return inventoryResources, err
		}

		inventoryResources = append(inventoryResources, secretRef)
	}

	if cd.Status.Inventory != nil {
		clustersToDelete := []client.Object{}
		for _, item := range cd.Status.Inventory.Entries {
			obj, err := unstructuredFromResourceRef(item)
			if err != nil {
				return inventoryResources, err
			}

			if obj.GetKind() == "GitopsCluster" {
				_, ok := clusterMapping[obj.GetName()]
				if !ok {
					clustersToDelete = append(clustersToDelete, obj)
				}
			}
		}

		for _, cluster := range clustersToDelete {
			if err := r.Client.Delete(ctx, cluster); err != nil {
				return inventoryResources, fmt.Errorf("failed to delete cluster: %w", err)
			}
		}
	}

	for _, ref := range existingClusters {
		objMeta, err := object.ParseObjMetadata(ref.ID)
		if err != nil {
			return inventoryResources, fmt.Errorf("failed to parse object ID %s: %w", ref.ID, err)
		}

		existingCluster := &gitopsv1alpha1.GitopsCluster{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: objMeta.Name, Namespace: objMeta.Namespace}, existingCluster); err != nil {
			return inventoryResources, fmt.Errorf("failed to load GitopsCluster for update: %w", err)
		}

		secretToUpdate := &corev1.Secret{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: existingCluster.Spec.SecretRef.Name, Namespace: cd.GetNamespace()}, secretToUpdate); err != nil {
			// TODO: don't error, create a new secret!
			return inventoryResources, fmt.Errorf("failed to get the secret to update: %w", err)
		}

		cluster := clusterMapping[existingCluster.GetName()]
		value, err := clientcmd.Write(*cluster.KubeConfig)
		if err != nil {
			return inventoryResources, err
		}
		secretToUpdate.Data["value"] = value
		// TODO: Patch!
		if err := r.Client.Update(ctx, secretToUpdate); err != nil {
			return inventoryResources, err
		}

	}

	return inventoryResources, nil
}

func (r *AutomatedClusterDiscoveryReconciler) patchStatus(ctx context.Context, req ctrl.Request, newStatus clustersv1alpha1.AutomatedClusterDiscoveryStatus) error {
	var set clustersv1alpha1.AutomatedClusterDiscovery
	if err := r.Get(ctx, req.NamespacedName, &set); err != nil {
		return err
	}

	patch := client.MergeFrom(set.DeepCopy())
	set.Status = newStatus

	return r.Status().Patch(ctx, &set, patch)
}

func newGitopsCluster(secretName string, name types.NamespacedName) *gitopsv1alpha1.GitopsCluster {
	return &gitopsv1alpha1.GitopsCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GitopsCluster",
			APIVersion: gitopsv1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: gitopsv1alpha1.GitopsClusterSpec{
			SecretRef: &meta.LocalObjectReference{
				Name: secretName,
			},
		},
	}
}

func newKubeConfigSecret(name types.NamespacedName, config *api.Config) (*corev1.Secret, error) {
	value, err := clientcmd.Write(*config)
	if err != nil {
		return nil, err
	}

	return newSecret(name, value), nil
}

func newSecret(name types.NamespacedName, value []byte) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Data: map[string][]byte{
			"value": value,
		},
	}
}

func unstructuredFromResourceRef(ref clustersv1alpha1.ResourceRef) (*unstructured.Unstructured, error) {
	objMeta, err := object.ParseObjMetadata(ref.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse object ID %s: %w", ref.ID, err)
	}
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(objMeta.GroupKind.WithVersion(ref.Version))
	u.SetName(objMeta.Name)
	u.SetNamespace(objMeta.Namespace)

	return &u, nil
}

func clustersToMapping(clusters []*providers.ProviderCluster) map[string]*providers.ProviderCluster {
	names := map[string]*providers.ProviderCluster{}
	for _, cluster := range clusters {
		names[cluster.Name] = cluster
	}

	return names
}

func labelsForResource(acd clustersv1alpha1.AutomatedClusterDiscovery) map[string]string {
	return map[string]string{
		k8sManagedByLabel:                       "cluster-reflector-controller",
		"clusters.weave.works/origin-name":      acd.GetName(),
		"clusters.weave.works/origin-namespace": acd.GetNamespace(),
		"clusters.weave.works/origin-type":      acd.Spec.Type,
	}
}
