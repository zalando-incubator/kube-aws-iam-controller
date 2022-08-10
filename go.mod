module github.com/zalando-incubator/kube-aws-iam-controller

require (
	github.com/aws/aws-sdk-go v1.16.6
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/heptiolabs/healthcheck v0.0.0-20211123025425-613501dd5deb
	github.com/prometheus/client_golang v1.13.0 // indirect
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.4.0
	gopkg.in/DATA-DOG/go-sqlmock.v1 v1.3.0 // indirect
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	k8s.io/api v0.18.3
	k8s.io/apimachinery v0.18.3
	k8s.io/client-go v0.18.3
)

replace k8s.io/klog => github.com/mikkeloscar/knolog v0.0.0-20190326191552-80742771eb6b

go 1.13
