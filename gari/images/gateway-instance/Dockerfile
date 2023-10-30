FROM golang:1.21.3 AS builder

WORKDIR /src
ADD go.mod go.sum /src
RUN CGO_ENABLED=0 go build sigs.k8s.io/controller-runtime/pkg/manager sigs.k8s.io/controller-runtime sigs.k8s.io/kubebuilder-declarative-pattern/commonclient  github.com/spiffe/go-spiffe/v2/workloadapi sigs.k8s.io/gateway-api/apis k8s.io/client-go/tools/clientcmd
ADD . /src
RUN CGO_ENABLED=0 go build -v -o /gateway-instance ./cmd/gateway-instance

FROM gcr.io/distroless/static-debian11:latest
COPY --from=builder /gateway-instance /gateway-instance
