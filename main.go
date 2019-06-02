package main

import (
	"context"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/mikkeloscar/kube-aws-iam-controller/pkg/clientset"
	log "github.com/sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	v1 "k8s.io/api/core/v1"
)

const (
	defaultInterval        = "10s"
	defaultRefreshLimit    = "15m"
	defaultEventQueueSize  = "10"
	defaultClientGOTimeout = 30 * time.Second
)

var (
	config struct {
		Debug          bool
		Interval       time.Duration
		RefreshLimit   time.Duration
		EventQueueSize int
		BaseRoleARN    string
		APIServer      *url.URL
		Namespace      string
		AssumeRoleARN  string
	}
)

func main() {
	kingpin.Flag("debug", "Enable debug logging.").BoolVar(&config.Debug)
	kingpin.Flag("interval", "Interval between syncing secrets.").
		Default(defaultInterval).DurationVar(&config.Interval)
	kingpin.Flag("refresh-limit", "Time limit when AWS IAM credentials should be refreshed. I.e. 15 min. before they expire.").
		Default(defaultRefreshLimit).DurationVar(&config.RefreshLimit)
	kingpin.Flag("event-queue-size", "Size of the pod event queue.").
		Default(defaultEventQueueSize).IntVar(&config.EventQueueSize)
	kingpin.Flag("base-role-arn", "Base Role ARN. If not defined it will be autodiscovered from EC2 Metadata.").
		StringVar(&config.BaseRoleARN)
	kingpin.Flag("assume-role-arn", "Assume Role ARN can be specified to assume a role at start-up which is used for further assuming other roles managed by the controller.").
		StringVar(&config.AssumeRoleARN)
	kingpin.Flag("namespace", "Limit the controller to a certain namespace.").
		Default(v1.NamespaceAll).StringVar(&config.Namespace)
	kingpin.Flag("apiserver", "API server url.").URLVar(&config.APIServer)
	kingpin.Parse()

	if config.Debug {
		log.SetLevel(log.DebugLevel)
	}

	ctx, cancel := context.WithCancel(context.Background())
	kubeConfig, err := clientset.ConfigureKubeConfig(config.APIServer, defaultClientGOTimeout, ctx.Done())
	if err != nil {
		log.Fatalf("Failed to setup Kubernetes config: %v", err)
	}

	client, err := clientset.NewForConfig(kubeConfig)
	if err != nil {
		log.Fatalf("Failed to initialize Kubernetes client: %v.", err)
	}

	awsSess, err := session.NewSession()
	if err != nil {
		log.Fatalf("Failed to setup AWS session: %v", err)
	}

	if config.BaseRoleARN == "" {
		config.BaseRoleARN, err = GetBaseRoleARN(awsSess)
		if err != nil {
			log.Fatalf("Failed to autodiscover Base Role ARN: %v", err)
		}

		log.Infof("Autodiscovered Base Role ARN: %s", config.BaseRoleARN)
	}

	awsConfigs := make([]*aws.Config, 0)
	if config.AssumeRoleARN != "" {
		creds := stscreds.NewCredentials(awsSess, config.AssumeRoleARN)
		awsConfigs = append(awsConfigs, &aws.Config{Credentials: creds})
	}

	credsGetter := NewSTSCredentialsGetter(awsSess, config.BaseRoleARN, awsConfigs...)

	podsEventCh := make(chan *PodEvent, config.EventQueueSize)

	controller := NewSecretsController(
		client,
		config.Namespace,
		config.Interval,
		config.RefreshLimit,
		credsGetter,
		podsEventCh,
	)

	podWatcher := NewPodWatcher(client, config.Namespace, podsEventCh)

	go handleSigterm(cancel)

	awsIAMRoleController := NewAWSIAMRoleController(
		client,
		config.Interval,
		config.RefreshLimit,
		credsGetter,
		config.Namespace,
	)

	go awsIAMRoleController.Run(ctx)

	podWatcher.Run(ctx)
	controller.Run(ctx)
}

// handleSigterm handles SIGTERM signal sent to the process.
func handleSigterm(cancelFunc func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	log.Info("Received Term signal. Terminating...")
	cancelFunc()
}
