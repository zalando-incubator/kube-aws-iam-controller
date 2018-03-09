package main

import (
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	defaultInterval          = "10s"
	defaultRefreshLimit      = "15m"
	defaultEventQueueSize    = "10"
	defaultIAMRoleAnnotation = "iam.amazonaws.com/role"
)

var (
	config struct {
		Interval          time.Duration
		RefreshLimit      time.Duration
		EventQueueSize    int
		BaseRoleARN       string
		APIServer         *url.URL
		IAMRoleAnnotation string
	}
)

func main() {
	kingpin.Flag("interval", "Interval between syncing secrets.").
		Default(defaultInterval).DurationVar(&config.Interval)
	kingpin.Flag("refresh-limit", "Time limit when AWS IAM credentials should be refreshed. I.e. 15 min. before they expire.").
		Default(defaultRefreshLimit).DurationVar(&config.RefreshLimit)
	kingpin.Flag("event-queue-size", "Size of the pod event queue.").
		Default(defaultEventQueueSize).IntVar(&config.EventQueueSize)
	kingpin.Flag("base-role-arn", "Base Role ARN. If not defined it will be autodiscovered from EC2 Metadata.").
		StringVar(&config.BaseRoleARN)
	kingpin.Flag("iam-role-annotation", "Name of the IAM Role annotation to be specified on pods. If the value is '' this feature will be disabled.").
		Default(defaultIAMRoleAnnotation).
		StringVar(&config.IAMRoleAnnotation)
	kingpin.Flag("apiserver", "API server url.").URLVar(&config.APIServer)
	kingpin.Parse()

	var kubeConfig *rest.Config

	if config.APIServer != nil {
		kubeConfig = &rest.Config{
			Host: config.APIServer.String(),
		}
	}

	client, err := kubeClient(kubeConfig)
	if err != nil {
		log.Fatalf("Failed to setup Kubernetes client: %v", err)
	}

	awsSess, err := session.NewSession()
	if err != nil {
		log.Fatalf("Failed to setup Kubernetes client: %v", err)
	}

	if config.BaseRoleARN == "" {
		config.BaseRoleARN, err = GetBaseRoleARN(awsSess)
		if err != nil {
			log.Fatalf("Failed to autodiscover Base Role ARN: %v", err)
		}

		log.Infof("Autodiscovered Base Role ARN: %s", config.BaseRoleARN)
	}

	if config.IAMRoleAnnotation != "" {
		log.Infof("Pod IAM Role annotation: %s", config.IAMRoleAnnotation)
	} else {
		log.Info("Pod IAM Role annotation discovery not enabled")
	}

	credsGetter := NewSTSCredentialsGetter(awsSess, config.BaseRoleARN)

	stopChs := make([]chan struct{}, 0, 2)
	podWatcherStopCh := make(chan struct{}, 1)
	stopChs = append(stopChs, podWatcherStopCh)
	controllerStopCh := make(chan struct{}, 1)
	stopChs = append(stopChs, controllerStopCh)

	podsEventCh := make(chan *PodEvent, config.EventQueueSize)

	controller := NewSecretsController(
		client,
		config.Interval,
		config.RefreshLimit,
		credsGetter,
		podsEventCh,
	)

	podWatcher := NewPodWatcher(client, podsEventCh, config.IAMRoleAnnotation)

	go handleSigterm(stopChs)

	podWatcher.Run(podWatcherStopCh)
	controller.Run(controllerStopCh)
}

func handleSigterm(stopChs []chan struct{}) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	log.Info("Received Term signal. Terminating...")
	for _, ch := range stopChs {
		close(ch)
	}
}

func kubeClient(config *rest.Config) (kubernetes.Interface, error) {
	var err error
	if config == nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}
