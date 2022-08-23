ARG BASE_IMAGE=registry.opensource.zalan.do/library/alpine-3:latest
FROM ${BASE_IMAGE}

ARG TARGETARCH

# add binary
COPY build/linux/${TARGETARCH}/kube-aws-iam-controller /

ENTRYPOINT ["/kube-aws-iam-controller"]
