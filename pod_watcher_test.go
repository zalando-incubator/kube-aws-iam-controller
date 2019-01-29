package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
)

func TestPodWatcherAdd(t *testing.T) {
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: secretPrefix + "my-app",
						},
					},
				},
			},
		},
	}

	events := make(chan *PodEvent, 1)
	watcher := NewPodWatcher(nil, v1.NamespaceAll, events)

	go watcher.add(pod)
	<-events

	// unexpected object type
	watcher.add("obj")
}

func TestPodWatcherDel(t *testing.T) {
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: secretPrefix + "my-app",
						},
					},
				},
			},
		},
	}

	events := make(chan *PodEvent, 1)
	watcher := NewPodWatcher(nil, v1.NamespaceAll, events)

	go watcher.del(pod)
	<-events

	// unexpected object type
	watcher.del("obj")
}

func TestIAMRole(t *testing.T) {
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: secretPrefix + "my-app",
						},
					},
				},
			},
		},
	}
	require.Equal(t, "my-app", iamRole(pod))

	pod = &v1.Pod{}
	require.Equal(t, "", iamRole(pod))
}
