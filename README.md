# AWS IAM Controller for Kubernetes
[![Build Status](https://travis-ci.org/mikkeloscar/kube-aws-iam-controller.svg?branch=master)](https://travis-ci.org/mikkeloscar/kube-aws-iam-controller)

This is an **experimental** Kubernetes controller for distributing AWS IAM role
credentials to pods via secrets.

It solves the same problem as other existing tools like
[jtblin/kube2iam](https://github.com/jtblin/kube2iam), namely distribute
different AWS IAM roles to different pods within the same cluster.  However, it
solves the problem in a different way to work around an inherit problem with
the architecture or kube2iam and similar solutions.

#### EC2 metadata service solution (kube2iam)

Kube2iam works by running an EC2 metadata service proxy on each node in order
to intercept role requests made by pods using one of the AWS SDKs. Instead of
forwarding the node IAM role to the pod, the proxy will make an assume role
call to [STS](https://docs.aws.amazon.com/STS/latest/APIReference/Welcome.html)
and get the role requested by the pod (via an annotation). If the assume role
request is fast, everything is fine, and the pod will get the correct role.
However, if the assume role request is too slow (>1s) then the AWS SDKs will
timeout and try to get credentials e.g. via the next option in the chain (e.g.
a file) resulting in the pod not getting the expected role.

This is often not a problem in clusters with a stable workload, but if you have
clusters with a very dynamic workload there will be a lot of cases where a pod
starts before kube2iam is ready to provide the expected role. One case is when
scaling up a cluster and a new pod lands on a fresh node before kube2iam,
another case is when a new pod is created and starts before kube2iam got the
event that the pod was created.

#### Kubernetes secrets solution

Instead of running as a proxy on each node, this controller runs as a
single instance and distributes AWS IAM credentials via secrets. This solves
the race condition problem by relying on a property of Kubernetes which ensures
that a secret, mounted by a pod, must exist before the pod is started.

One minor trade-off with this solution is that each pod requiring AWS IAM
credentials must define a secret mount rather than a single annotation.

**NB** This approach currently doesn't work out of the box as the AWS SDKs
doesn't support refreshing credentials from a file.

## How it works

The controller watches for new pods, if it sees a pod which has an AWS IAM role
defined it will ensure there is a secret containing credentials for the IAM
role which can be mounted as a file by the pod.

Each secret resource created by the controller will have a label
`heritage=kube-aws-iam-controller` to indicate that it's owned by the
controller.
The controller will continuously pull all secrets with this label and ensure
the credentials are refreshed before they expire. It will also cleanup secrets
with credentials which are no longer requested by any pods.

### Specifying AWS IAM role on pods

In order to specify that a pod should get a certain AWS IAM role assigned the
pod spec must include a volume mount from a secret with the following secret
name pattern: `aws-iam-<you-iam-role-name>`. Further more a volume mount point
must be defined for each container requiring the role in the pod and each
container must also have the environment variable
`AWS_SHARED_CREDENTIALS_FILE=/path/to/mounted/secret` defined. The environment
variable is used by AWS SDKs and the AWS CLI to automatically find and use the
credentials file.

See a full example in [example-app.yaml](/docs/example-app.yaml).

**Note**: This way of specifying the role on pod specs are subject to change.
It is currently moving a lot of effort on to the users defining the pod specs.
A future idea is to make the controller act as an admission controller which
can inject the required configuration automatically.

## Setup

The `kube-aws-iam-controller` can be run as a deployment in the cluster.
See [deployment.yaml](/docs/deployment.yaml).

Deploy it by running:

```bash
$ kubectl apply -f docs/deployment.yaml
```

To ensure that pods requiring AWS IAM roles doesn't go to the EC2 metadata
service of the node instead of using the credentials file provided by the
secret you must block the metadata service from the pod network on each node.
E.g. with an iptables rule:

```bash
$ /usr/sbin/iptables \
      --append PREROUTING \
      --protocol tcp \
      --destination 169.254.169.254 \
      --dport 80 \
      --in-interface cni0 \
      --match tcp \
      --jump DROP
```

Where `cni0` is the interface of the pod network on the node.

**Note**: The controller will read all pods on startup and therefor the memory
limit for the pod must be set relative to the number of pods in the cluster
(i.e. vertical scaling).

### Bootstrap in non-AWS environment

If you need access to AWS from another environment e.g. GKE then the controller
can be deployed with seed credentials and refresh its own credentials used
for the assume role calls similar to how it refreshes all other credentials.

To create the initial seed credentials you must configure an AWS IAM role used
for the assume role calls. In this example the IAM role is created via
cloudformation, but you can do it however you like. The important part is that
the role has permissions to do `sts` calls as it will be assuming other roles.
And you should also allow the role to be assumed by your own user for creating
the initial seed credentials:

```sh
$ cat role.yaml
Metadata:
  StackName: kube-aws-iam-controller-role
Resources:
  KubeAWSIAMControllerRole:
    Type: AWS::IAM::Role
    Properties:
      RoleName: kube-aws-iam-controller-role
      AssumeRolePolicyDocument:
        Version: '2012-10-17'
        Statement:
        - Action: ['sts:AssumeRole']
          Effect: Allow
          Principal:
            AWS: "<arn-of-your-user>"
        Version: '2012-10-17'
      Path: /
      Policies:
      - PolicyName: assumer-role
        PolicyDocument:
          Version: '2012-10-17'
          Statement:
          - Action:
            - sts:*
            Resource: "*"
            Effect: Allow
# create the role via cloudformation
$ aws cloudformation create-stack --stack-name kube-aws-iam-controller-role --template-body=file://role.yaml --capabilities CAPABILITY_NAMED_IAM
```

And then you can use the script `./scripts/get_credentials.sh` to generate
initial credentials and create a secret.

```sh
$ export ARN="arn.of.the.iam.role"
$ kubectl create secret generic aws-iam-<name-of-role> --from-literal "credentials=$(./scripts/get_credentials.sh "$ARN")"
```

Once the secret is created you can deploy the controller using the example
manifest in [deployment_with_role.yaml](/docs/deployment_with_role.yaml).

The controller will use the secret you created with temporary credentials and
continue to refresh the credentials automatically.

## Building

In order to build you first get the dependencies which are managed by
[dep](https://github.com/golang/dep):

```bash
$ go get -u github.com/golang/dep/cmd/dep
$ dep ensure -vendor-only # install all dependencies
```

After dependencies are installed the controller can be built simply by running:

```bash
$ make
```
