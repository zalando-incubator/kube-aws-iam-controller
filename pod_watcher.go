package main

import (
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	initializerName                = "iam.amazonaws.com"
	awsIAMCredentialsMountPath     = "/meta/aws-iam"
	awsSharedCredentialsFileEnvVar = "AWS_SHARED_CREDENTIALS_FILE"
	awsIAMCredentialsVolumeName    = "aws-iam-credentials"
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
	client            kubernetes.Interface
	podEvents         chan<- *PodEvent
	iamRoleAnnotation string
}

// NewPodWatcher initializes a new PodWatcher.
func NewPodWatcher(client kubernetes.Interface, podEvents chan<- *PodEvent, iamRoleAnnotation string) *PodWatcher {
	return &PodWatcher{
		client:            client,
		podEvents:         podEvents,
		iamRoleAnnotation: iamRoleAnnotation,
	}
}

// Run setups up a shared informer for listing and watching changes to pods and
// starts listening for events.
func (p *PodWatcher) Run(stopCh <-chan struct{}) {
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

	go informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, informer.HasSynced) {
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
	}

	err := p.initializePod(pod)
	if err != nil {
		log.Errorf("Failed to initialize Pod: %v", err)
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

// TODO: check if env/mounts exists before adding them
func (p *PodWatcher) initializePod(pod *v1.Pod) error {
	log.Infof("Handling pod: %s/%s", pod.Namespace, pod.Name)

	if pod.Initializers != nil &&
		len(pod.Initializers.Pending) > 0 &&
		pod.Initializers.Pending[0].Name == initializerName {
		log.Infof("To be initialized!")

		role := ""

		// get requested IAM role name from annotation
		if p.iamRoleAnnotation != "" {
			role = pod.Annotations[p.iamRoleAnnotation]
		}

		if role == "" {
			// TODO: get role from service account
			// https://github.com/mikkeloscar/kube-aws-iam-controller/issues/1
		}

		if role != "" {
			// add volume mounts and environment variables
			for i, container := range pod.Spec.Containers {
				env := v1.EnvVar{
					Name:  awsSharedCredentialsFileEnvVar,
					Value: awsIAMCredentialsMountPath + "/credentials",
				}
				container.Env = append(container.Env, env)

				mount := v1.VolumeMount{
					Name:      awsIAMCredentialsVolumeName,
					MountPath: awsIAMCredentialsMountPath,
					ReadOnly:  true,
				}
				container.VolumeMounts = append(container.VolumeMounts, mount)

				pod.Spec.Containers[i] = container
			}

			// add secret volume
			volume := v1.Volume{
				Name: awsIAMCredentialsVolumeName,
				VolumeSource: v1.VolumeSource{
					Secret: &v1.SecretVolumeSource{
						SecretName: secretPrefix + role,
					},
				},
			}
			pod.Spec.Volumes = append(pod.Spec.Volumes, volume)
		}

		pending := make([]metav1.Initializer, 0, len(pod.Initializers.Pending)-1)
		for _, init := range pod.Initializers.Pending {
			if init.Name != initializerName {
				pending = append(pending, init)
			}
		}

		pod.Initializers.Pending = pending

		// update pod
		// TODO: retries
		_, err := p.client.CoreV1().Pods(pod.Namespace).Update(pod)
		return err
	}

	return nil
}
