package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	log "github.com/sirupsen/logrus"
	"github.com/zalando-incubator/kube-aws-iam-controller/pkg/clientset"
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
		Debug               bool
		Interval            time.Duration
		RefreshLimit        time.Duration
		EventQueueSize      int
		BaseRoleARN         string
		APIServer           *url.URL
		Namespace           string
		AssumeRole          string
		UseRegionalEndpoint bool
	}
)

type STSEndpoint struct {
	Endpoint string
}

// NewSTSCredentialsGetter initializes a new STS based credentials fetcher.
func NewSTSEndpoint() *STSEndpoint {
	return &STSEndpoint{
		Endpoint: "sts.amazonaws.com",
	}
}
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
	kingpin.Flag("assume-role", "Assume Role can be specified to assume a role at start-up which is used for further assuming other roles managed by the controller.").
		StringVar(&config.AssumeRole)
	kingpin.Flag("namespace", "Limit the controller to a certain namespace.").
		Default(v1.NamespaceAll).StringVar(&config.Namespace)
	kingpin.Flag("apiserver", "API server url.").URLVar(&config.APIServer)
	kingpin.Flag("use-regional-sts-endpoint", "Use the regional sts endpoint if AWS_DEFAULT_REGION is set").BoolVar(&config.UseRegionalEndpoint)
	kingpin.Parse()

	if config.Debug {
		log.SetLevel(log.DebugLevel)
	}

	ctx, cancel := context.WithCancel(context.Background())
	kubeConfig, err := clientset.ConfigureKubeConfig(config.APIServer, defaultClientGOTimeout, ctx.Done())
	if err != nil {
		log.Fatalf("Failed to set up Kubernetes config: %v", err)
	}

	client, err := clientset.NewForConfig(kubeConfig)
	if err != nil {
		log.Fatalf("Failed to initialize Kubernetes client: %v.", err)
	}

	awsSess, err := session.NewSession()
	if err != nil {
		log.Fatalf("Failed to set up AWS session: %v", err)
	}

	if config.BaseRoleARN == "" {
		config.BaseRoleARN, err = GetBaseRoleARN(awsSess)
		if err != nil {
			log.Fatalf("Failed to autodiscover Base Role ARN: %v", err)
		}

		log.Infof("Autodiscovered Base Role ARN: %s", config.BaseRoleARN)
	}

	baseRoleARNPrefix, err := GetPrefixFromARN(config.BaseRoleARN)
	if err != nil {
		log.Fatalf("Failed to parse ARN prefix from Base Role ARN: %v", err)
	}
	log.Debugf("Parsed Base Role ARN prefix: %s", baseRoleARNPrefix)

	awsConfigs := aws.NewConfig()
	if config.AssumeRole != "" {
		if !strings.HasPrefix(config.AssumeRole, baseRoleARNPrefix) {
			config.AssumeRole = config.BaseRoleARN + config.AssumeRole
		}
		log.Infof("Using custom Assume Role: %s", config.AssumeRole)
		creds := stscreds.NewCredentials(awsSess, config.AssumeRole)
		awsConfigs = awsConfigs.WithCredentials(creds)
	}

	if config.UseRegionalEndpoint {
		awsConfigs = awsConfigs.WithEndpointResolver(NewSTSEndpoint())
	}

	credsGetter := NewSTSCredentialsGetter(awsSess, config.BaseRoleARN, baseRoleARNPrefix, awsConfigs)

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

// GetEndpointFromRegion formas a standard sts endpoint url given a region
func GetEndpointFromRegion(region string) string {
	endpoint := fmt.Sprintf("https://sts.%s.amazonaws.com", region)
	if strings.HasPrefix(region, "cn-") {
		endpoint = fmt.Sprintf("https://sts.%s.amazonaws.com.cn", region)
	}
	return endpoint
}

// IsValidRegion tests for a vaild region name
func IsValidRegion(promisedLand string) bool {
	partitions := endpoints.DefaultResolver().(endpoints.EnumPartitions).Partitions()
	for _, p := range partitions {
		for region := range p.Regions() {
			if promisedLand == region {
				return true
			}
		}
	}
	return false
}

// EndpointFor implements the endpoints.Resolver interface for use with sts
func (e *STSEndpoint) EndpointFor(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
	// only for sts service
	if service == "sts" {
		// only if a valid region is explicitly set
		if IsValidRegion(region) {
			e.Endpoint = GetEndpointFromRegion(region)
			return endpoints.ResolvedEndpoint{
				URL:           e.Endpoint,
				SigningRegion: region,
			}, nil
		}
	}
	return endpoints.DefaultResolver().EndpointFor(service, region, optFns...)
}

// handleSigterm handles SIGTERM signal sent to the process.
func handleSigterm(cancelFunc func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	log.Info("Received Term signal. Terminating...")
	cancelFunc()
}
