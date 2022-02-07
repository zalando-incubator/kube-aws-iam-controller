FROM registry.opensource.zalan.do/library/alpine-3.13:latest

# add binary
COPY build/linux/kube-aws-iam-controller /

ENTRYPOINT ["/kube-aws-iam-controller"]
