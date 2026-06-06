# Minimal, network-free runtime image for the example service.
# Build the static binary on the host first (the example is in the same module):
#
#   GOWORK=off CGO_ENABLED=0 GOOS=linux go build -o observability/demo/app ./example/metrics
#
# Then build the image straight into minikube's container runtime (scratch needs no pull):
#
#   minikube image build -t healthcheck-example:dev observability/demo
#
# The Deployment uses imagePullPolicy: Never so it consumes this local image.
FROM scratch
COPY app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
