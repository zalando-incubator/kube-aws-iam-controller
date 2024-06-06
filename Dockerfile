ARG BASE_IMAGE=registry.opensource.zalan.do/library/static:latest
FROM ${BASE_IMAGE}

ARG TARGETARCH

# add binary
COPY build/linux/${TARGETARCH}/kube-aws-iam-controller /

ENTRYPOINT ["/kube-aws-iam-controller"]
