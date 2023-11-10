package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/meta"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ReconciliationFailedReason represents the fact that
	// the reconciliation failed.
	ReconciliationFailedReason string = "ReconciliationFailed"

	// ReconciliationSucceededReason represents the fact that
	// the reconciliation succeeded.
	ReconciliationSucceededReason string = "ReconciliationSucceeded"
)

// SetAutomatedClusterDiscoveryReadiness sets the ready condition with the given status, reason and message.
func SetAutomatedClusterDiscoveryReadiness(acd *AutomatedClusterDiscovery, inventory *ResourceInventory, status metav1.ConditionStatus, reason, message string) {
	if inventory != nil {
		acd.Status.Inventory = inventory

		if len(inventory.Entries) == 0 {
			acd.Status.Inventory = nil
		}
	}

	acd.Status.ObservedGeneration = acd.ObjectMeta.Generation
	newCondition := metav1.Condition{
		Type:    meta.ReadyCondition,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
	apimeta.SetStatusCondition(&acd.Status.Conditions, newCondition)
}

// GetAutomatedClusterDiscoveryReadiness returns the readiness condition of the AutomatedClusterDiscovery.
func GetAutomatedClusterDiscoveryReadiness(acd *AutomatedClusterDiscovery) metav1.ConditionStatus {
	return apimeta.FindStatusCondition(acd.Status.Conditions, meta.ReadyCondition).Status
}
