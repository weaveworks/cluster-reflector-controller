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
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/predicates"
	gitopsv1alpha1 "github.com/weaveworks/cluster-controller/api/v1alpha1"
	clustersv1alpha1 "github.com/weaveworks/cluster-reflector-controller/api/v1alpha1"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers/aws"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers/azure"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers/capi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cli-utils/pkg/object"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const k8sManagedByLabel = "app.kubernetes.io/managed-by"

type eventRecorder interface {
	Event(object runtime.Object, eventtype, reason, message string)
}

type providerFactoryFunc func(client.Reader, *clustersv1alpha1.AutomatedClusterDiscovery) (providers.Provider, error)

// AutomatedClusterDiscoveryReconciler reconciles a AutomatedClusterDiscovery object
type AutomatedClusterDiscoveryReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder eventRecorder

	ProviderFactory providerFactoryFunc
}

// event emits a Kubernetes event and forwards the event to the event recorder
func (r *AutomatedClusterDiscoveryReconciler) event(obj *clustersv1alpha1.AutomatedClusterDiscovery, eventtype, reason, message string) {
	r.EventRecorder.Event(obj, eventtype, reason, message)
}

//+kubebuilder:rbac:groups=clusters.weave.works,resources=automatedclusterdiscoveries,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=clusters.weave.works,resources=automatedclusterdiscoveries/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=clusters.weave.works,resources=automatedclusterdiscoveries/finalizers,verbs=update
//+kubebuilder:rbac:groups=gitops.weave.works,resources=gitopsclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

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
		logger.Info("reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	logger.Info("reconciling cluster reflector",
		"type", clusterDiscovery.Spec.Type,
	)

	// Set the value of the reconciliation request in status.
	if v, ok := meta.ReconcileAnnotationValue(clusterDiscovery.GetAnnotations()); ok {
		clusterDiscovery.Status.LastHandledReconcileAt = v
		if err := r.patchStatus(ctx, req, clusterDiscovery.Status); err != nil {
			return ctrl.Result{}, err
		}
	}

	inventoryRefs, err := r.reconcileResources(ctx, clusterDiscovery)
	if err != nil {
		clustersv1alpha1.SetAutomatedClusterDiscoveryReadiness(clusterDiscovery, clusterDiscovery.Status.Inventory, metav1.ConditionFalse, clustersv1alpha1.ReconciliationFailedReason, err.Error())

		if err := r.patchStatus(ctx, req, clusterDiscovery.Status); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	clusterDiscovery.Status.Inventory = &clustersv1alpha1.ResourceInventory{Entries: inventoryRefs}

	// Get number of clusters in inventory
	clusters := 0
	for _, item := range inventoryRefs {
		objMeta, err := object.ParseObjMetadata(item.ID)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to parse object ID %s: %w", item.ID, err)
		}

		if objMeta.GroupKind.Kind == "GitopsCluster" {
			clusters++
		}
	}

	if inventoryRefs != nil {
		logger.Info("reconciled clusters", "count", len(inventoryRefs))
		clustersv1alpha1.SetAutomatedClusterDiscoveryReadiness(clusterDiscovery, clusterDiscovery.Status.Inventory, metav1.ConditionTrue, clustersv1alpha1.ReconciliationSucceededReason,
			fmt.Sprintf("%d clusters discovered", clusters))

		if err = r.patchStatus(ctx, req, clusterDiscovery.Status); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		logger.Info("no clusters to reconcile")
	}

	interval := clusterDiscovery.Spec.Interval
	if interval == (metav1.Duration{}) {
		interval = metav1.Duration{Duration: time.Minute * 5}
	}

	return ctrl.Result{RequeueAfter: interval.Duration}, nil
}

func (r *AutomatedClusterDiscoveryReconciler) reconcileResources(ctx context.Context, cd *clustersv1alpha1.AutomatedClusterDiscovery) ([]clustersv1alpha1.ResourceRef, error) {
	logger := log.FromContext(ctx)

	provider, err := r.ProviderFactory(r.Client, cd)
	if err != nil {
		logger.Error(err, "failed to create provider", "type", cd.Spec.Type)
		return nil, err
	}

	clusters, err := provider.ListClusters(ctx)
	if err != nil {
		logger.Error(err, "failed to list clusters from provider", "type", cd.Spec.Type)
		return nil, err
	}

	clusterID, err := provider.ClusterID(ctx, r.Client)
	if err != nil {
		logger.Error(err, "failed to get cluster ID", "type", cd.Spec.Type)
		return nil, err
	}

	// TODO: Fix this so that we record the inventoryRefs even if we get an
	// error.
	inventoryRefs, err := r.reconcileClusters(ctx, clusters, clusterID, cd)
	if err != nil {
		logger.Error(err, "failed to reconcile clusters")
		return nil, err
	}

	sort.Slice(inventoryRefs, func(i, j int) bool {
		return inventoryRefs[i].ID < inventoryRefs[j].ID
	})

	return inventoryRefs, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutomatedClusterDiscoveryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clustersv1alpha1.AutomatedClusterDiscovery{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicates.ReconcileRequestedPredicate{}))).
		Owns(&gitopsv1alpha1.GitopsCluster{}, builder.MatchEveryOwner).
		Complete(r)
}

func (r *AutomatedClusterDiscoveryReconciler) reconcileClusters(ctx context.Context, clusters []*providers.ProviderCluster, currentClusterID string, acd *clustersv1alpha1.AutomatedClusterDiscovery) ([]clustersv1alpha1.ResourceRef, error) {
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
		if currentClusterID != "" && currentClusterID == cluster.ID {
			logger.Info("skipping current cluster")
			continue
		}
		secretName := fmt.Sprintf("%s-kubeconfig", cluster.Name)

		gitopsCluster := newGitopsCluster(types.NamespacedName{
			Name:      cluster.Name,
			Namespace: acd.Namespace,
		})

		clusterRef, err := clustersv1alpha1.ResourceRefFromObject(gitopsCluster)
		if err != nil {
			return inventoryResources, err
		}

		if isExistingRef(clusterRef, acd.Status.Inventory) {
			existingClusters = append(existingClusters, clusterRef)
		}

		logger.Info("creating gitops cluster", "name", gitopsCluster.GetName())
		if err := controllerutil.SetOwnerReference(acd, gitopsCluster, r.Scheme); err != nil {
			return inventoryResources, fmt.Errorf("failed to set ownership on created GitopsCluster: %w", err)
		}

		clusterLabels := labelsForResource(*acd)
		if !acd.Spec.DisableTags {
			clusterLabels = mergeMaps(cluster.Labels, clusterLabels)
		}

		gitopsCluster.SetLabels(clusterLabels)

		gitopsCluster.SetAnnotations(mergeMaps(acd.Spec.CommonAnnotations, map[string]string{
			gitopsv1alpha1.GitOpsClusterNoSecretFinalizerAnnotation: "true",
		}))
		_, err = controllerutil.CreateOrPatch(ctx, r.Client, gitopsCluster, func() error {
			if acd.Spec.Type == "aks" {
				gitopsCluster.Spec = gitopsv1alpha1.GitopsClusterSpec{
					SecretRef: &meta.LocalObjectReference{
						Name: secretName,
					},
				}
			} else if acd.Spec.Type == "capi" {
				gitopsCluster.Spec = gitopsv1alpha1.GitopsClusterSpec{
					CAPIClusterRef: &meta.LocalObjectReference{
						Name: cluster.Name,
					},
				}
			}

			return nil
		})
		if err != nil {
			return inventoryResources, err
		}

		inventoryResources = append(inventoryResources, clusterRef)

		if cluster.KubeConfig != nil {
			secretRef, err := r.createSecret(ctx, secretName, acd.Namespace, gitopsCluster, acd, cluster)
			if err != nil {
				return inventoryResources, err
			}

			inventoryResources = append(inventoryResources, *secretRef)
		}

		// publish event for ClusterCreated
		r.event(acd, corev1.EventTypeNormal, "ClusterCreated", fmt.Sprintf("Cluster %s created", cluster.Name))

	}

	if acd.Status.Inventory != nil {
		clustersToDelete := []client.Object{}
		for _, item := range acd.Status.Inventory.Entries {
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
			logger.Info("deleting gitops cluster", "name", cluster.GetName())
			if err := r.Client.Delete(ctx, cluster); err != nil {
				return inventoryResources, fmt.Errorf("failed to delete cluster: %w", err)
			}

			// publish event for ClusterRemoved
			r.event(acd, corev1.EventTypeNormal, "ClusterRemoved", fmt.Sprintf("Cluster %s removed", cluster.GetName()))
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

		if existingCluster.Spec.SecretRef != nil {
			secretToUpdate := &corev1.Secret{}
			if err := r.Client.Get(ctx, types.NamespacedName{Name: existingCluster.Spec.SecretRef.Name, Namespace: acd.GetNamespace()}, secretToUpdate); err != nil {
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

func (r *AutomatedClusterDiscoveryReconciler) createSecret(ctx context.Context, secretName, namespace string, gitopsCluster *gitopsv1alpha1.GitopsCluster, acd *clustersv1alpha1.AutomatedClusterDiscovery, cluster *providers.ProviderCluster) (*clustersv1alpha1.ResourceRef, error) {
	logger := log.FromContext(ctx)
	secret := newSecret(types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	})

	secretRef, err := clustersv1alpha1.ResourceRefFromObject(secret)
	if err != nil {
		return nil, err
	}

	logger.Info("creating secret", "name", secret.GetName())
	if err := controllerutil.SetOwnerReference(gitopsCluster, secret, r.Scheme); err != nil {
		logger.Error(err, "failed to set ownership on created Secret")
		return nil, fmt.Errorf("failed to set ownership on created Secret: %w", err)
	}

	secret.SetLabels(labelsForResource(*acd))
	secret.SetAnnotations(acd.Spec.CommonAnnotations)

	// publish event for ClusterCreated
	r.event(acd, corev1.EventTypeNormal, "ClusterCreated", fmt.Sprintf("Cluster %s created", cluster.Name))
	_, err = controllerutil.CreateOrPatch(ctx, r.Client, secret, func() error {
		value, err := clientcmd.Write(*cluster.KubeConfig)
		if err != nil {
			return err
		}

		secret.Data["value"] = value

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &secretRef, nil
}

func newGitopsCluster(name types.NamespacedName) *gitopsv1alpha1.GitopsCluster {
	return &gitopsv1alpha1.GitopsCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GitopsCluster",
			APIVersion: gitopsv1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
	}
}

func newSecret(name types.NamespacedName) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Data: map[string][]byte{},
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
	appliedLabels := map[string]string{
		k8sManagedByLabel:                       "cluster-reflector-controller",
		"clusters.weave.works/origin-name":      acd.GetName(),
		"clusters.weave.works/origin-namespace": acd.GetNamespace(),
		"clusters.weave.works/origin-type":      acd.Spec.Type,
	}

	return mergeMaps(acd.Spec.CommonLabels, appliedLabels)
}

func mergeMaps[K comparable, V any](maps ...map[K]V) map[K]V {
	result := map[K]V{}

	for _, map_ := range maps {
		if map_ == nil {
			continue
		}

		for k, v := range map_ {
			result[k] = v
		}
	}

	return result
}

// DefaultProviderFactory creates an appropriate factory for creating provider
// clients based on the spec of the AutomatedClusterDiscovery.
func DefaultProviderFactory(k8sClient client.Reader, acd *clustersv1alpha1.AutomatedClusterDiscovery) (providers.Provider, error) {
	switch acd.Spec.Type {
	case "aks":
		if acd.Spec.AKS == nil {
			return nil, errors.New("discovery .spec.type = aks but no AKS configuration provided")
		}

		return azure.NewAzureProvider(acd.Spec.AKS.SubscriptionID), nil
	case "eks":
		if acd.Spec.EKS == nil {
			return nil, errors.New("discovery .spec.type = eks but no EKS configuration provided")
		}

		return aws.NewAWSProvider(acd.Spec.EKS.Region), nil

	case "capi":
		return capi.NewCAPIProvider(k8sClient, acd.GetNamespace(), acd.Spec.CAPI.CurrentClusterRef), nil
	}

	return nil, fmt.Errorf("unknown provider type: %s", acd.Spec.Type)
}
