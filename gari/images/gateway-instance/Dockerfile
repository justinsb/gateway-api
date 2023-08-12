FROM golang:1.20.7 AS builder

WORKDIR /src
ADD go.mod go.sum /src
RUN CGO_ENABLED=0 go build sigs.k8s.io/controller-runtime/pkg/manager sigs.k8s.io/kubebuilder-declarative-pattern/commonclient
ADD . /src
RUN CGO_ENABLED=0 go build -v -o /gateway-instance ./cmd/gateway-instance

FROM gcr.io/distroless/static-debian11:latest
COPY --from=builder /gateway-instance /gateway-instance
