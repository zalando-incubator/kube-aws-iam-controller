package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AWSIAMRole describes an AWS IAM Role for which credentials can be
// provisioned in a cluster.
// +k8s:deepcopy-gen=true
type AWSIAMRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AWSIAMRoleSpec   `json:"spec"`
	Status AWSIAMRoleStatus `json:"status"`
}

// AWSIAMRoleSpec is the spec part of the AWSIAMRole resource.
// +k8s:deepcopy-gen=true
type AWSIAMRoleSpec struct {
	RoleReference       string `json:"roleReference"`
	RoleSessionDuration int64  `json:"roleSessionDuration"`
}

// AWSIAMRoleStatus is the status section of the AWSIAMRole resource.
// resource.
// +k8s:deepcopy-gen=true
type AWSIAMRoleStatus struct {
	// observedGeneration is the most recent generation observed for this
	// AWSIAMRole. It corresponds to the AWSIAMRole's generation, which is
	// updated on mutation by the API Server.
	// +optional
	ObservedGeneration *int64       `json:"observedGeneration,omitempty" protobuf:"varint,1,opt,name=observedGeneration"`
	RoleARN            string       `json:"roleARN"`
	Expiration         *metav1.Time `json:"expiration"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AWSIAMRoleList is a list of AWSIAMRoles.
// +k8s:deepcopy-gen=true
type AWSIAMRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AWSIAMRole `json:"items"`
}
