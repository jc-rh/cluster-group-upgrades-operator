/*
Copyright 2021.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RemediationStrategySpec defines the remediation policy
type RemediationStrategySpec struct {
	// Canaries defines the list of managed clusters that should be remediated first when remediateAction is set to enforce
	Canaries       []string `json:"canaries,omitempty"`
	MaxConcurrency int      `json:"maxConcurrency,omitempty"`
}

// DesiredUpdateSpec models the desiredUpdate field of ClusterVersion
type DesiredUpdateSpec struct {
	Version string `json:"version,omitempty"`
	Image   string `json:"image,omitempty"`
	Force   bool   `json:"force,omitempty"`
}

// OperatorUpgradeSpec defines the configuration of an operator upgrade
type OperatorUpgradeSpec struct {
	Channel   string `json:"channel,omitempty"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// PlatformUpgradeSpec defines the configuration of a platform upgrade
type PlatformUpgradeSpec struct {
	Channel       string            `json:"channel,omitempty"`
	Upstream      string            `json:"upstream,omitempty"`
	DesiredUpdate DesiredUpdateSpec `json:"desiredUpdate,omitempty"`
}

// ClusterGroupUpgradeSpec defines the desired state of ClusterGroupUpgrade
//+kubebuilder:printcolumn:name="Compliance Percentage",type="integer",JSONPath=".status.status.compliancePercentage"
type ClusterGroupUpgradeSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Clusters            []string                 `json:"clusters,omitempty"`
	RemediationStrategy *RemediationStrategySpec `json:"remediationStrategy,omitempty"`
	RemediationAction   string                   `json:"remediationAction,omitempty"`
	PlatformUpgrade     *PlatformUpgradeSpec     `json:"platformUpgrade,omitempty"`
	OperatorUpgrades    []OperatorUpgradeSpec    `json:"operatorUpgrades,omitempty"`
}

// UpgradeStatus defines the observed state of the upgrade
type UpgradeStatus struct {
	CompliancePercentage    int            `json:"compliancePercentage"`
	PlatformUpgradePolicies []PolicyStatus `json:"platformUpgradesPolicies,omitempty"`
	OperatorUpgradePolicies []PolicyStatus `json:"operatorUpgradePolicies,omitempty"`
}

// PolicyStatus defines the observed state of a Policy
type PolicyStatus struct {
	Name            string `json:"name,omitempty"`
	ComplianceState string `json:"complianceState,omitempty"`
}

// ClusterGroupUpgradeStatus defines the observed state of ClusterGroupUpgrade
type ClusterGroupUpgradeStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	PlacementBindings []string           `json:"placementBindings,omitempty"`
	PlacementRules    []string           `json:"placementRules,omitempty"`
	Policies          []string           `json:"policies,omitempty"`
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	RemediationPlan   [][]string         `json:"remediationPlan,omitempty"`
	Status            UpgradeStatus      `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ClusterGroupUpgrade is the Schema for the ClusterGroupUpgrades API
type ClusterGroupUpgrade struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterGroupUpgradeSpec   `json:"spec,omitempty"`
	Status ClusterGroupUpgradeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterGroupUpgradeList contains a list of ClusterGroupUpgrade
type ClusterGroupUpgradeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterGroupUpgrade `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterGroupUpgrade{}, &ClusterGroupUpgradeList{})
}