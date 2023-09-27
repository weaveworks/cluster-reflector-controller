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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clustersv1alpha1 "weave.works/cluster-reflector-controller/api/v1alpha1"
)

// AutomatedClusterDiscoveryReconciler reconciles a AutomatedClusterDiscovery object
type AutomatedClusterDiscoveryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=clusters.weave.works,resources=automatedclusterdiscoveries,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=clusters.weave.works,resources=automatedclusterdiscoveries/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=clusters.weave.works,resources=automatedclusterdiscoveries/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AutomatedClusterDiscovery object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.0/pkg/reconcile
func (r *AutomatedClusterDiscoveryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	clusterDiscovery := &clustersv1alpha1.AutomatedClusterDiscovery{}
	if err := r.Get(ctx, req.NamespacedName, clusterDiscovery); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling cluster reflector",
		"type", clusterDiscovery.Spec.Type,
		"name", clusterDiscovery.Spec.Name,
	)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutomatedClusterDiscoveryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clustersv1alpha1.AutomatedClusterDiscovery{}).
		Complete(r)
}
