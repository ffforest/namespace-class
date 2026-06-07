FROM scratch

ARG BINARY=bin/linux-arm64/namespace-class-controller

COPY ${BINARY} /namespace-class-controller

USER 65532:65532
ENTRYPOINT ["/namespace-class-controller"]
