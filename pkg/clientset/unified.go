package clientset

import (
	"net"
	"net/http"
	"net/url"
	"time"

	awsiamrole "github.com/zalando-incubator/kube-aws-iam-controller/pkg/client/clientset/versioned"
	zalandov1 "github.com/zalando-incubator/kube-aws-iam-controller/pkg/client/clientset/versioned/typed/zalando.org/v1"
	"k8s.io/client-go/kubernetes"
	rest "k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
)

type Interface interface {
	kubernetes.Interface
	ZalandoV1() zalandov1.ZalandoV1Interface
}

type Clientset struct {
	kubernetes.Interface
	awsiamrole awsiamrole.Interface
}

func NewClientset(kubernetes kubernetes.Interface, awsiamrole awsiamrole.Interface) *Clientset {
	return &Clientset{
		kubernetes,
		awsiamrole,
	}
}

func NewForConfig(kubeconfig *rest.Config) (*Clientset, error) {
	kubeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	awsIAMRoleClient, err := awsiamrole.NewForConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	return &Clientset{kubeClient, awsIAMRoleClient}, nil
}

func (c *Clientset) ZalandoV1() zalandov1.ZalandoV1Interface {
	return c.awsiamrole.ZalandoV1()
}

// ConfigureKubeConfig configures a kubeconfig.
func ConfigureKubeConfig(apiServerURL *url.URL, timeout time.Duration, stopCh <-chan struct{}) (*rest.Config, error) {
	tr := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
			DualStack: false, // K8s do not work well with IPv6
		}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       20 * time.Second,
	}

	// We need this to reliably fade on DNS change, which is right
	// now not fixed with IdleConnTimeout in the http.Transport.
	// https://github.com/golang/go/issues/23427
	go func(d time.Duration) {
		for {
			select {
			case <-time.After(d):
				tr.CloseIdleConnections()
			case <-stopCh:
				return
			}
		}
	}(20 * time.Second)

	if apiServerURL != nil {
		return &rest.Config{
			Host:      apiServerURL.String(),
			Timeout:   timeout,
			Transport: tr,
			QPS:       100.0,
			Burst:     500,
		}, nil
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	// patch TLS config
	restTransportConfig, err := config.TransportConfig()
	if err != nil {
		return nil, err
	}
	restTLSConfig, err := transport.TLSConfigFor(restTransportConfig)
	if err != nil {
		return nil, err
	}
	tr.TLSClientConfig = restTLSConfig

	config.Timeout = timeout
	config.Transport = tr
	config.QPS = 100.0
	config.Burst = 500
	// disable TLSClientConfig to make the custom Transport work
	config.TLSClientConfig = rest.TLSClientConfig{}
	return config, nil
}
