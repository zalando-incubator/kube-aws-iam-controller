#!/bin/bash
set -o errexit -o pipefail

: ${NAMESPACE:=kube-system}
: ${ROLE_NAME:=kube-aws-iam-controller-iam-role}

if [[ $# != 1 ]]; then
  echo "usage: $0 <kube-aws-iam-controller-role-arn>" 1>&2
  exit 1
fi
role_arn="$1"

role_uid="$(kubectl get -n "$NAMESPACE" awsiamrole "$ROLE_NAME" -o 'jsonpath={.metadata.uid}')"
credentials_json="$(aws sts assume-role --role-arn "$role_arn" --role-session-name kube-aws-iam-controller | jq '.Credentials + {"Version": 1}' | base64 -w0)"
credentials_process="$(base64 -w0 <<<$'[default]\ncredential_process = cat /meta/aws-iam/credentials.json')"

kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: $ROLE_NAME
  namespace: $NAMESPACE
  labels:
    heritage: kube-aws-iam-controller
    type: awsiamrole
  ownerReferences:
    - apiVersion: zalando.org/v1
      kind: AWSIAMRole
      name: $ROLE_NAME
      uid: $role_uid
type: Opaque
data:
  credentials.json: $credentials_json
  credentials.process: $credentials_process
EOF
