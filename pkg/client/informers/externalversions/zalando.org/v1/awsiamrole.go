/*
Copyright 2025 The Kubernetes Authors.

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

// Code generated by informer-gen. DO NOT EDIT.

package v1

import (
	context "context"
	time "time"

	apiszalandoorgv1 "github.com/zalando-incubator/kube-aws-iam-controller/pkg/apis/zalando.org/v1"
	versioned "github.com/zalando-incubator/kube-aws-iam-controller/pkg/client/clientset/versioned"
	internalinterfaces "github.com/zalando-incubator/kube-aws-iam-controller/pkg/client/informers/externalversions/internalinterfaces"
	zalandoorgv1 "github.com/zalando-incubator/kube-aws-iam-controller/pkg/client/listers/zalando.org/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// AWSIAMRoleInformer provides access to a shared informer and lister for
// AWSIAMRoles.
type AWSIAMRoleInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() zalandoorgv1.AWSIAMRoleLister
}

type aWSIAMRoleInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewAWSIAMRoleInformer constructs a new informer for AWSIAMRole type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewAWSIAMRoleInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredAWSIAMRoleInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredAWSIAMRoleInformer constructs a new informer for AWSIAMRole type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredAWSIAMRoleInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ZalandoV1().AWSIAMRoles(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ZalandoV1().AWSIAMRoles(namespace).Watch(context.TODO(), options)
			},
		},
		&apiszalandoorgv1.AWSIAMRole{},
		resyncPeriod,
		indexers,
	)
}

func (f *aWSIAMRoleInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredAWSIAMRoleInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *aWSIAMRoleInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&apiszalandoorgv1.AWSIAMRole{}, f.defaultInformer)
}

func (f *aWSIAMRoleInformer) Lister() zalandoorgv1.AWSIAMRoleLister {
	return zalandoorgv1.NewAWSIAMRoleLister(f.Informer().GetIndexer())
}
