# AWS IAM Controller for Kubernetes
[![Build Status](https://travis-ci.org/mikkeloscar/kube-aws-iam-controller.svg?branch=master)](https://travis-ci.org/mikkeloscar/kube-aws-iam-controller)
[![Coverage Status](https://coveralls.io/repos/github/mikkeloscar/kube-aws-iam-controller/badge.svg?branch=master)](https://coveralls.io/github/mikkeloscar/kube-aws-iam-controller?branch=master)

This is a **Proof of Concept** Kubernetes controller for distributing AWS IAM
role credentials to pods via secrets.

It aims to solve the same problem as other existing tools like
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
timeout and try to get credentials via the next option in the chain (e.g.
a file) resulting in the pod not getting the expected role or no role at all.

This is often not a problem in clusters with a stable workload, but if you have
clusters with a very dynamic workload there will be a lot of cases where a pod
starts before kube2iam is ready to provide the expected role. One case is when
scaling up a cluster and a new pod lands on a fresh node before kube2iam,
another case is when a new pod is created and starts before kube2iam got the
event that the pod was created.
During update of the kube2iam daemonset there will also be a short timeframe
where the metadata url will be unavailable for the pods which could lead to a
refresh of credentials failing.

#### Kubernetes secrets solution

Instead of running as a proxy on each node, this controller runs as a
single instance and distributes AWS IAM credentials via secrets. This solves
the race condition problem by relying on a property of Kubernetes which ensures
that a secret, mounted by a pod, must exist before the pod is started. This
means that the controller can even be away for a few minutes without affecting
pods running in the cluster as they will still be able to mount and read the
secrets.
Furthermore having a single controller means there is only one caller for to
the AWS API resulting in fewer calls which can prevent ratelimiting in big
clusters and you don't need to give all nodes the power to assume other roles
if it's not needed.

One minor trade-off with this solution is that each pod requiring AWS IAM
credentials must define a secret mount rather than a single annotation.

**NB** This approach currently only works for some of the AWS SDKs. I'm
reaching out to AWS to figure out if this is something that could be supported.

See the [configuration guide for supported SDKs](/docs/sdk-configuration.md).

## How it works

The controller continuously looks for custom `AWSIAMRole` resources which
specify an AWS IAM role by name or by the full ARN. For each resource it finds,
it will generate/update corresponding secrets containing credentialds for the
IAM role specified.
The secrets can be mounted by pods as a file enabling the
AWS SDKs to use the credentials.

If an `AWSIAMRole` resource is deleted, the corresponding secret would be
automatically cleaned up as well.

### Specifying AWS IAM role on pods

**See the [configuration guide for supported
SDKs](/docs/sdk-configuration.md)**.

In order to specify that a certain AWS IAM Role should be available for
applications in a namespace you need to define an `AWSIAMRole` resource which
references the IAM role you want:

```yaml
apiVersion: zalando.org/v1
kind: AWSIAMRole
metadata:
  name: my-app-iam-role
spec:
  # The roleReference allows specifying an AWS IAM role name or arn
  # Possible values:
  #   "aws-iam-role-name"
  #   "arn:aws:iam::<account-id>:role/aws-iam-role-name"
  roleReference: <my-iam-role-name-or-arn>
```

The controller will detect the resource and create a corresponding secret with
the same name containing the role credentials. To use the credentials in a pod
you simply mount the secret (called `my-app-iam-role` in this example), making
the credentials available as a file for your application to read and use.
Additionally you must also define an environment variable
`AWS_SHARED_CREDENTIALS_FILE=/path/to/mounted/secret` for each container. The
environment variable is used by AWS SDKs and the AWS CLI to automatically find
and use the credentials file.

See a full example in [example-app.yaml](/docs/example-app.yaml).

**Note**: This way of specifying the role on pod specs are subject to change.
It is currently moving a lot of effort on to the users defining the pod specs.
A future idea is to make the controller act as an admission controller which
can inject the required configuration automatically.

### Setting up AWS IAM roles

The controller does not take care of AWS IAM role provisioning and assumes that
the user provisions AWS IAM roles manually, for instance via
[CloudFormation](https://aws.amazon.com/cloudformation/) or
[Terraform](https://www.terraform.io/).

Here is an example of an AWS IAM role defined via CloudFormation:

```yaml
Parameters:
  AssumeRoleARN: 
    Description: "Role ARN of the role used by kube-aws-iam-controller"
    Type: String
Metadata:
  StackName: "aws-iam-example"
AWSTemplateFormatVersion: "2010-09-09"
Description: "Example IAM Role"
Resources:
  IAMRole:
    Type: AWS::IAM::Role
    Properties:
      RoleName: "aws-iam-example"
      Path: /
      AssumeRolePolicyDocument:
        Statement:
        - Action: sts:AssumeRole
          Effect: Allow
          Principal:
            AWS: !Ref "AssumeRoleARN"
        Version: '2012-10-17'
      Policies:
      - PolicyName:  "policy"
        PolicyDocument:
          Version: '2012-10-17'
          Statement:
          - Effect: Allow
            Action:
            - "ec2:Describe*"
            Resource: "*"
```

The role could be created via:

```sh
# $ASSUME_ROLE_ARN is the ARN of the role used by the kube-aws-iam-controller deployment
$ aws cloudformation create-stack --stack-name aws-iam-example \
  --parameters "ParameterKey=AssumeRoleARN,ParameterValue=$ASSUME_ROLE_ARN" \
  --template-body=file://iam-role.yaml --capabilities CAPABILITY_NAMED_IAM
```

The important part is the `AssumeRolePolicyDocument`:

```yaml
AssumeRolePolicyDocument:
  Statement:
  - Action: sts:AssumeRole
    Effect: Allow
    Principal:
      AWS: !Ref "AssumeRoleARN"
  Version: '2012-10-17'
```

This allows the `kube-aws-iam-controller` to assume the role and provide
credentials on behalf of the application requesting credentials via an
`AWSIAMRole` resource in the cluster.

The `AssumeRoleARN` is the ARN of the role which the `kube-aws-iam-controller`
is running with. Usually this would be the instance role of the EC2 instance
were the controller is running.

#### Using custom Assume role

Sometimes it's desirable to let the controller assume roles with a specific
role dedicated for that task i.e. a role different from the instance role. The
controller allows specifying such a role via the
`--assume-role=<controller-role>` flag providing the following setup:

```
                                                                           +-------------+
                                                                           |             |
                                                                      +--> | <app-role1> |
+-----------------+                +-------------------+              |    |             |
|                 |                |                   |              |    +-------------+
| <instance-role> | -- assumes --> | <controller-role> | -- assumes --+
|                 |                |                   |              |    +-------------+
+-----------------+                +-------------------+              |    |             |
                                                                      +--> | <app-role2> |
                                                                           |             |
                                                                           +-------------+
```

In this case the `<instance-role>` will only be used for the initial assuming
of the `<controller-role>` and all `<app-role>s` are assumed by the
`<controller-role>`. This makes it possible to have many different
`<instance-role>s` while the `<app-role>s` only have to trust the single static
`<controller-role>`. If you don't specify `--assume-role` then the
`<instance-role>` would have to assume the `<app-role>s`.

Here is an example of the AWS IAM roles defined for this set-up to work:

```yaml
Metadata:
  StackName: "aws-iam-assume-role-example"
AWSTemplateFormatVersion: "2010-09-09"
Description: "Example AWS IAM Assume Role"
Resources:
  InstanceIAMRole:
    Type: AWS::IAM::Role
    Properties:
      RoleName: "instance-role"
      Path: /
      AssumeRolePolicyDocument:
        Statement:
        - Action: sts:AssumeRole
          Effect: Allow
          Principal:
            Service: ec2.amazonaws.com
        Version: '2012-10-17'
      Policies:
      - PolicyName:  "policy"
        PolicyDocument:
          Version: '2012-10-17'
          Statement:
          - Effect: Allow
            Action:
            - "sts:AssumeRole"
            Resource: "*"

  KubeAWSIAMControllerIAMRole:
    Type: AWS::IAM::Role
    Properties:
      RoleName: "kube-aws-iam-controller"
      Path: /
      AssumeRolePolicyDocument:
        Statement:
        - Action: sts:AssumeRole
          Effect: Allow
          Principal:
            AWS: !Sub 'arn:${AWS::Partition}:iam::${AWS::AccountId}:role/${InstanceIAMRole}'
        Version: '2012-10-17'
      Policies:
      - PolicyName:  "policy"
        PolicyDocument:
          Version: '2012-10-17'
          Statement:
          - Effect: Allow
            Action:
            - "sts:AssumeRole"
            Resource: "*"

  APPIAMRole:
    Type: AWS::IAM::Role
    Properties:
      RoleName: "app-role"
      Path: /
      AssumeRolePolicyDocument:
        Statement:
        - Action: sts:AssumeRole
          Effect: Allow
          Principal:
            AWS: !Sub 'arn:${AWS::Partition}:iam::${AWS::AccountId}:role/${KubeAWSIAMControllerIAMRole}'
        Version: '2012-10-17'
      Policies:
      - PolicyName:  "policy"
        PolicyDocument:
          Version: '2012-10-17'
          Statement:
          - Effect: Allow
            Action:
            - "ec2:Describe*"
            Resource: "*"
```

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
$ kubectl create secret generic kube-aws-iam-controller-iam-role --from-literal "credentials.json=$(./scripts/get_credentials.sh "$ARN")" --from-literal "credentials.process=$(printf "[default]\ncredential_process = cat /meta/aws-iam/credentials.json\n")"
```

Once the secret is created you can deploy the controller using the example
manifest in [deployment_with_role.yaml](/docs/deployment_with_role.yaml).

The controller will use the secret you created with temporary credentials and
continue to refresh the credentials automatically.

## Building

This project uses [Go modules](https://github.com/golang/go/wiki/Modules) as
introduced in Go 1.11 therefore you need Go >=1.11 installed in order to build.
If using Go 1.11 you also need to [activate Module
support](https://github.com/golang/go/wiki/Modules#installing-and-activating-module-support).

Assuming Go has been setup with module support it can be built simply by running:

```sh
export GO111MODULE=on # needed if the project is checked out in your $GOPATH.
$ make
```
