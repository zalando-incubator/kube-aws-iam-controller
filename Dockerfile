FROM container-registry.zalando.net/library/alpine-3.13:latest

ARG TARGETARCH

# add binary
COPY build/linux/${TARGETARCH}/kube-aws-iam-controller /

ENTRYPOINT ["/kube-aws-iam-controller"]
