FROM registry.opensource.zalan.do/stups/alpine:latest

# add binary
COPY build/linux/kube-aws-iam-controller /

ENTRYPOINT ["/kube-aws-iam-controller"]
