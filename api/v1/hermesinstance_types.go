/*
Copyright 2026 stubbi.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HermesInstanceSpec defines the desired state of HermesInstance.
type HermesInstanceSpec struct {
	// Image controls which hermes-agent container image to run.
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// Storage controls the PVC backing ~/.hermes for this instance.
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`
}

// ImageSpec selects an OCI image.
type ImageSpec struct {
	// +kubebuilder:default="ghcr.io/stubbi/hermes-agent"
	// +optional
	Repository string `json:"repository,omitempty"`

	// +kubebuilder:default="latest"
	// +optional
	Tag string `json:"tag,omitempty"`

	// +kubebuilder:default=IfNotPresent
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +optional
	PullPolicy string `json:"pullPolicy,omitempty"`
}

// StorageSpec controls the PVC backing the agent's data directory.
type StorageSpec struct {
	Persistence PersistenceSpec `json:"persistence,omitempty"`
}

type PersistenceSpec struct {
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// +kubebuilder:default="1Gi"
	// +optional
	Size string `json:"size,omitempty"`

	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// HermesInstanceStatus reflects the observed state of HermesInstance.
type HermesInstanceStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase is a short human-readable status (Pending|Ready|Degraded).
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the instance's state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=hi;hermes,categories=hermes;agents
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image.repository`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// HermesInstance is the Schema for the hermesinstances API
type HermesInstance struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of HermesInstance
	// +required
	Spec HermesInstanceSpec `json:"spec"`

	// status defines the observed state of HermesInstance
	// +optional
	Status HermesInstanceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// HermesInstanceList contains a list of HermesInstance
type HermesInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []HermesInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HermesInstance{}, &HermesInstanceList{})
}
