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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AKS defines the desired state of AKS
type AKS struct {
	// SubscriptionID is the Azure subscription ID
	// +required
	SubscriptionID string `json:"subscriptionID"`

	Filter AKSFilter `json:"filter,omitempty"`

	// Exclude is the list of clusters to exclude
	Exclude []string `json:"exclude,omitempty"`
}

// Filter criteria for AKS clusters
type AKSFilter struct {
	// Location is the location of the AKS clusters
	Location string `json:"location,omitempty"`
}

// AutomatedClusterDiscoverySpec defines the desired state of AutomatedClusterDiscovery
type AutomatedClusterDiscoverySpec struct {
	// Name is the name of the cluster
	Name string `json:"name,omitempty"`

	// Type is the provider type
	// +kubebuilder:validation:Enum=aks
	Type string `json:"type"`

	AKS *AKS `json:"aks,omitempty"`

	// The interval at which to run the discovery
	// +required
	Interval metav1.Duration `json:"interval"`
}

// AutomatedClusterDiscoveryStatus defines the observed state of AutomatedClusterDiscovery
type AutomatedClusterDiscoveryStatus struct {
	// Inventory contains the list of Kubernetes resource object references that
	// have been successfully applied
	// +optional
	Inventory *ResourceInventory `json:"inventory,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AutomatedClusterDiscovery is the Schema for the automatedclusterdiscoveries API
type AutomatedClusterDiscovery struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutomatedClusterDiscoverySpec   `json:"spec,omitempty"`
	Status AutomatedClusterDiscoveryStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AutomatedClusterDiscoveryList contains a list of AutomatedClusterDiscovery
type AutomatedClusterDiscoveryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutomatedClusterDiscovery `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AutomatedClusterDiscovery{}, &AutomatedClusterDiscoveryList{})
}
