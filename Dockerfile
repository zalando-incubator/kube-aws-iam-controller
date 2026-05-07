ARG BASE_IMAGE=gcr.io/distroless/static-debian12:latest
FROM ${BASE_IMAGE}

ARG TARGETARCH

# add binary
COPY build/linux/${TARGETARCH}/kube-aws-iam-controller /

ENTRYPOINT ["/kube-aws-iam-controller"]
