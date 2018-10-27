package main

import (
	"context"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// PodEvent defines an event triggered when a pod is created/updated/deleted.
type PodEvent struct {
	Role      string
	Name      string
	Namespace string
	Deletion  bool
}

// PodWatcher lists and watches for changes to pods.
type PodWatcher struct {
	client    kubernetes.Interface
	podEvents chan<- *PodEvent
}

// NewPodWatcher initializes a new PodWatcher.
func NewPodWatcher(client kubernetes.Interface, podEvents chan<- *PodEvent) *PodWatcher {
	return &PodWatcher{
		client:    client,
		podEvents: podEvents,
	}
}

// Run setups up a shared informer for listing and watching changes to pods and
// starts listening for events.
func (p *PodWatcher) Run(ctx context.Context) {
	informer := cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(p.client.CoreV1().RESTClient(), "pods", v1.NamespaceAll, fields.Everything()),
		&v1.Pod{},
		0, // skip resync
		cache.Indexers{},
	)

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    p.add,
		DeleteFunc: p.del,
	})

	go informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		log.Errorf("Timed out waiting for caches to sync")
		return
	}

	log.Info("Synced Pod watcher")
}

// add sends an add pod event to the podEvents queue.
func (p *PodWatcher) add(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		log.Errorf("Failed to get pod object")
		return
	}

	role := iamRole(pod)

	if role != "" {
		event := &PodEvent{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Role:      role,
		}
		p.podEvents <- event
	}
}

// add sends a delete pod event to the podEvents queue.
func (p *PodWatcher) del(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		log.Errorf("Failed to get pod object")
		return
	}

	role := iamRole(pod)

	if role != "" {
		event := &PodEvent{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Role:      role,
			Deletion:  true,
		}
		p.podEvents <- event
	}
}

// iamRole tries to find an AWS IAM role name by looking into the secret name
// of volumes defined for the pod.
func iamRole(pod *v1.Pod) string {
	for _, volume := range pod.Spec.Volumes {
		if volume.Secret != nil && strings.HasPrefix(volume.Secret.SecretName, secretPrefix) {
			return strings.TrimPrefix(volume.Secret.SecretName, secretPrefix)
		}
	}
	return ""
}
