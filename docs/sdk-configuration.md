# Configure credentials for SDKs

The AWS SDKs does not have complete feature parity and therefore must be
configured slightly different to receive credentials in Kubernetes.

Below is a support matrix for the different SDKs along with a configuration
guide for those that have support already.

| SDK | Supported | Version | Comment |
| --- | --------- | ------- | ------- |
| [Java AWS SDK (JVM)](#java-aws-sdk-jvm) | :heavy_check_mark: | `>=1.11.394`, `>=2.7.8` | Configuration differs slightly between v1 and v2 of the SDK |
| [Python AWS SDK (boto3)](#python-aws-sdk-boto3) | :heavy_check_mark: | `>=1.9.28` | |
| [AWS CLI](#aws-cli) | :heavy_check_mark: | `>=1.16.43` | |
| [Ruby AWS SDK](#) | :heavy_plus_sign: | | Supported but not yet tested ([aws-sdk-ruby/#1820](https://github.com/aws/aws-sdk-ruby/pull/1820)) |
| [Golang AWS SDK](#golang-aws-sdk) | :heavy_check_mark: | `>=v1.16.2` | |
| [JS AWS SDK](#) | :heavy_multiplication_x: | | Not yet supported ([aws-sdk-js/#1923](https://github.com/aws/aws-sdk-js/pull/1923)) |

## Java AWS SDK (JVM)

| SDK | Tested version |
|-----| -------------- |
| [aws-sdk-java](https://github.com/aws/aws-sdk-java) | `>=1.11.394` |
| [aws-sdk-java-v2](https://github.com/aws/aws-sdk-java-v2) | `>=2.7.8` |

Here's a minimal example of how to configure a deployment so each pod will get
the AWS credentials.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aws-iam-java-example
spec:
  replicas: 1
  selector:
    matchLabels:
      application: aws-iam-java-example
  template:
    metadata:
      labels:
        application: aws-iam-java-example
    spec:
      containers:
      - name: aws-iam-java-example
        image: mikkeloscar/kube-aws-iam-controller-java-example:latest
        env:
        # must be set for the Java AWS SDK/AWS CLI to find the credentials file if you use the AWS SDK for Java v1
        - name: AWS_CREDENTIAL_PROFILES_FILE
          value: /meta/aws-iam/credentials
        # must be set for the Java AWS SDK/AWS CLI to find the credentials file if you use the AWS SDK for Java v2
        - name: AWS_SHARED_CREDENTIALS_FILE
          value: /meta/aws-iam/credentials.process
        volumeMounts:
        - name: aws-iam-credentials
          mountPath: /meta/aws-iam
          readOnly: true
      volumes:
      - name: aws-iam-credentials
        secret:
          secretName: aws-iam-java-example # name of the AWSIAMRole resource
---
apiVersion: zalando.org/v1
kind: AWSIAMRole
metadata:
  name: aws-iam-java-example
spec:
  roleReference: aws-iam-example
```

It's important that you set the `AWS_CREDENTIALS_PROFILES_FILE` or `AWS_SHARED_CREDENTIALS_FILE` depending on whether you use version 1 or version 2 of the Java AWS SDK (see the [AWS SDK for Java migration guide](https://docs.aws.amazon.com/sdk-for-java/v2/migration-guide/client-credential.html) for more info).
You also need to mount the secret named after the `AWSIAMRole` resource into the pod under `/meta/aws-iam`. This secret will be provisioned by the **kube-aws-iam-controller**.

See full [Java example project](https://github.com/mikkeloscar/kube-aws-iam-controller-java-example).

## Python AWS SDK (boto3)

| SDK | Tested version |
|-----| -------------- |
| [boto3](https://github.com/boto/boto3) | `>=1.9.28` |

Here's a minimal example of how to configure a deployment so each pod will get
the AWS credentials.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aws-iam-python-example
spec:
  replicas: 1
  selector:
    matchLabels:
      application: aws-iam-python-example
  template:
    metadata:
      labels:
        application: aws-iam-python-example
    spec:
      containers:
      - name: aws-iam-python-example
        image: mikkeloscar/kube-aws-iam-controller-python-example:latest
        env:
        # must be set for the AWS SDK/AWS CLI to find the credentials file.
        - name: AWS_SHARED_CREDENTIALS_FILE # used by python SDK
          value: /meta/aws-iam/credentials.process
        - name: AWS_DEFAULT_REGION # adjust to your AWS region
          value: eu-central-1
        volumeMounts:
        - name: aws-iam-credentials
          mountPath: /meta/aws-iam
          readOnly: true
      volumes:
      - name: aws-iam-credentials
        secret:
          secretName: aws-iam-python-example # name of the AWSIAMRole resource
---
apiVersion: zalando.org/v1
kind: AWSIAMRole
metadata:
  name: aws-iam-python-example
spec:
  roleReference: aws-iam-example
```

It's important that you set the `AWS_SHARED_CREDENTIALS_FILE` environment
variable as shown in the example as well as mounting the secret named after the
`AWSIAMRole` resource into the pod under `/meta/aws-iam`. This
secret will be provisioned by the **kube-aws-iam-controller**.

Also note that for this to work the docker image you use **MUST** contain the
program `cat`. [`cat` is called by the SDK to read the credentials from a
file](https://docs.aws.amazon.com/cli/latest/topic/config-vars.html#sourcing-credentials-from-external-processes).

See full [Python example project](https://github.com/mikkeloscar/kube-aws-iam-controller-python-example).

## AWS CLI

| SDK | Tested version |
|-----| -------------- |
| [aws-cli](https://github.com/aws/aws-cli) | `>=1.16.43` |

Configuration is the same as for the [Python AWS SDK](#python-aws-sdk-boto3).

## Golang AWS SDK

| SDK | Minimal version |
|-----| -------------- |
| [aws-sdk-go](https://github.com/aws/aws-sdk-go) | `>=v1.16.2` |

Here's a minimal example of how to configure a deployment so each pod will get
the AWS credentials.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aws-iam-golang-example
spec:
  replicas: 1
  selector:
    matchLabels:
      application: aws-iam-golang-example
  template:
    metadata:
      labels:
        application: aws-iam-golang-example
    spec:
      containers:
      - name: aws-iam-golang-example
        image: mikkeloscar/kube-aws-iam-controller-golang-example:latest
        env:
        # must be set for the AWS SDK/AWS CLI to find the credentials file.
        - name: AWS_SHARED_CREDENTIALS_FILE # used by golang SDK
          value: /meta/aws-iam/credentials.process
        - name: AWS_REGION # adjust to your AWS region
          value: eu-central-1
        volumeMounts:
        - name: aws-iam-credentials
          mountPath: /meta/aws-iam
          readOnly: true
      volumes:
      - name: aws-iam-credentials
        secret:
          secretName: aws-iam-golang-example # name of the AWSIAMRole resource
---
apiVersion: zalando.org/v1
kind: AWSIAMRole
metadata:
  name: aws-iam-golang-example
spec:
  roleReference: aws-iam-example
```

It's important that you set the `AWS_SHARED_CREDENTIALS_FILE` environment
variable as shown in the example as well as mounting the secret named after the
`AWSIAMRole` resource into the pod under `/meta/aws-iam`. This secret will be
provisioned by the **kube-aws-iam-controller**.

Also note that for this to work the docker image you use **MUST** contain the
program `cat`. [`cat` is called by the SDK to read the credentials from a
file](https://docs.aws.amazon.com/cli/latest/topic/config-vars.html#sourcing-credentials-from-external-processes).

Additionally it's important that your application initializes the AWS session
using the
[`session.NewSession()`](https://docs.aws.amazon.com/sdk-for-go/api/aws/session/#NewSession)
function which correctly initializes the credentials chain. Using the
DEPRECATED `session.New()` will **NOT** work!

See full [Golang example project](https://github.com/mikkeloscar/kube-aws-iam-controller-golang-example).
