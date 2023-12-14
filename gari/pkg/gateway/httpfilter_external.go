package gateway

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	pb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type externalFilter struct {
	client pb.ExternalProcessorClient
}

var _ Filter = &oidcAuthFilter{}

//+kubebuilder:rbac:groups=gari.gateway.networking.x-k8s.io,resources=external,verbs=get;list;watch

func buildExternalFilter(ctx context.Context, client client.Client, ns string, ref *gatewayapi.LocalObjectReference) (*externalFilter, error) {
	// obj := &v1alpha1.External{}
	// id := types.NamespacedName{
	// 	Name:      string(ref.Name),
	// 	Namespace: ns,
	// }
	// if ref.Name == "" {
	// 	return nil, fmt.Errorf("name not set")
	// }
	// if err := client.Get(ctx, id, obj); err != nil {
	// 	return nil, fmt.Errorf("getting oidcauth object: %w", err)
	// }

	// TODO: Make generic
	target := "kweb-sso-gateway-filter.kweb-sso-system:80"

	dialOptions := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	conn, err := grpc.Dial(target, dialOptions...)
	if err != nil {
		return nil, fmt.Errorf("dialing grpc target %q: %w", target, err)
	}
	externalProcessorClient := pb.NewExternalProcessorClient(conn)

	return &externalFilter{
		client: externalProcessorClient,
	}, nil
}

func (f *externalFilter) Handle(w http.ResponseWriter, req *http.Request) bool {
	ctx := req.Context()

	log := klog.FromContext(ctx)

	// TODO: Time limits on context
	stream, err := f.client.Process(ctx)
	if err != nil {
		klog.Warningf("error starting external process stream: %v", err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return true
	}

	defer func() {
		if err := stream.CloseSend(); err != nil {
			log.Error(err, "error closing stream")
		}
	}()

	headers := &corev3.HeaderMap{}
	for k, values := range req.Header {
		k = strings.ToLower(k)

		switch k {
		case ":method", ":path", ":scheme":
			continue
		case "x-forwarded-client-cert":
			continue
		}

		for _, v := range values {
			headers.Headers = append(headers.Headers, &corev3.HeaderValue{
				Key:   k,
				Value: v,
			})
		}
	}
	headers.Headers = append(headers.Headers, &corev3.HeaderValue{Key: ":method", Value: req.Method})
	headers.Headers = append(headers.Headers, &corev3.HeaderValue{Key: ":path", Value: req.URL.Path})
	headers.Headers = append(headers.Headers, &corev3.HeaderValue{Key: ":scheme", Value: req.URL.Scheme})
	headers.Headers = append(headers.Headers, &corev3.HeaderValue{Key: ":host", Value: req.Host})

	var xfccValues []string
	if req.TLS != nil {
		for _, peerCertificate := range req.TLS.PeerCertificates {
			log.Info("peerCertificate", "cert", peerCertificate)
			pem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: peerCertificate.Raw})
			xfcc := "Cert=" + url.QueryEscape(string(pem))
			xfcc += fmt.Sprintf(";Subject=%q", peerCertificate.Subject.String())
			xfccValues = append(xfccValues, xfcc)
		}
	}
	headers.Headers = append(headers.Headers, &corev3.HeaderValue{Key: "x-forwarded-client-cert", Value: strings.Join(xfccValues, ",")})

	headersRequest := &pb.ProcessingRequest_RequestHeaders{
		RequestHeaders: &pb.HttpHeaders{
			Headers: headers,
			// EndOfStream: true,
		},
	}

	if err := stream.Send(&pb.ProcessingRequest{Request: headersRequest}); err != nil {
		klog.Warningf("error sending headers: %v", err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return true
	}

	headersResponse, err := stream.Recv()
	if err != nil {
		klog.Warningf("error getting headers response: %v", err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return true
	}

	log.Info("headers response", "response", headersResponse)
	switch r := headersResponse.Response.(type) {
	case *pb.ProcessingResponse_ImmediateResponse:
		for _, h := range r.ImmediateResponse.GetHeaders().GetSetHeaders() {
			w.Header().Add(h.GetHeader().Key, h.GetHeader().Value)
		}

		statusCode := int(r.ImmediateResponse.GetStatus().GetCode())
		if statusCode == 0 {
			statusCode = 200
		}
		w.WriteHeader(statusCode)

		// TODO: Write body?

		return true

	case *pb.ProcessingResponse_RequestHeaders:
		for _, h := range r.RequestHeaders.GetResponse().GetHeaderMutation().GetSetHeaders() {
			log.Info("setting header", "key", h.GetHeader().Key, "value", h.GetHeader().Value)
			req.Header.Add(h.GetHeader().Key, h.GetHeader().Value)
		}

		return false

	case nil:
		// No special handling
	default:
		klog.Warningf("unhandled response type %T", r)
	}

	return false
}
