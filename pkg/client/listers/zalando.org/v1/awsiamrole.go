/*
Copyright 2024 The Kubernetes Authors.

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

// Code generated by lister-gen. DO NOT EDIT.

package v1

import (
	v1 "github.com/zalando-incubator/kube-aws-iam-controller/pkg/apis/zalando.org/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/listers"
	"k8s.io/client-go/tools/cache"
)

// AWSIAMRoleLister helps list AWSIAMRoles.
// All objects returned here must be treated as read-only.
type AWSIAMRoleLister interface {
	// List lists all AWSIAMRoles in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.AWSIAMRole, err error)
	// AWSIAMRoles returns an object that can list and get AWSIAMRoles.
	AWSIAMRoles(namespace string) AWSIAMRoleNamespaceLister
	AWSIAMRoleListerExpansion
}

// aWSIAMRoleLister implements the AWSIAMRoleLister interface.
type aWSIAMRoleLister struct {
	listers.ResourceIndexer[*v1.AWSIAMRole]
}

// NewAWSIAMRoleLister returns a new AWSIAMRoleLister.
func NewAWSIAMRoleLister(indexer cache.Indexer) AWSIAMRoleLister {
	return &aWSIAMRoleLister{listers.New[*v1.AWSIAMRole](indexer, v1.Resource("awsiamrole"))}
}

// AWSIAMRoles returns an object that can list and get AWSIAMRoles.
func (s *aWSIAMRoleLister) AWSIAMRoles(namespace string) AWSIAMRoleNamespaceLister {
	return aWSIAMRoleNamespaceLister{listers.NewNamespaced[*v1.AWSIAMRole](s.ResourceIndexer, namespace)}
}

// AWSIAMRoleNamespaceLister helps list and get AWSIAMRoles.
// All objects returned here must be treated as read-only.
type AWSIAMRoleNamespaceLister interface {
	// List lists all AWSIAMRoles in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.AWSIAMRole, err error)
	// Get retrieves the AWSIAMRole from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1.AWSIAMRole, error)
	AWSIAMRoleNamespaceListerExpansion
}

// aWSIAMRoleNamespaceLister implements the AWSIAMRoleNamespaceLister
// interface.
type aWSIAMRoleNamespaceLister struct {
	listers.ResourceIndexer[*v1.AWSIAMRole]
}
