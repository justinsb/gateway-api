package gateway

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
)

type Filter interface {
	// Handle is called to handle the filter.
	// If the filter returns true, request processing is stopped and the filter is considered to have "handled" the request.
	// If the filter returns false, request processing continues to the next filter.
	Handle(ctx context.Context, httpRequest *HTTPRequest) bool
}

type errorFilter struct {
	err error
}

func (f *errorFilter) Handle(ctx context.Context, httpRequest *HTTPRequest) bool {
	log := klog.FromContext(ctx)

	log.Error(f.err, "filter error")
	http.Error(httpRequest.w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
	return true
}

func (r *httpRule) buildFilter(ctx context.Context, client client.Client, ns string, obj *gatewayapi.HTTPRouteFilter) (Filter, error) {
	switch obj.Type {
	case gatewayapi.HTTPRouteFilterRequestRedirect:
		if obj.RequestRedirect == nil {
			return nil, fmt.Errorf("requestRedirect not set in filter %v", obj)
		}
		return buildRequestRedirectFilter(obj.RequestRedirect)
	// case gatewayapi.HTTPRouteFilterExtensionRef:
	// 	if obj.ExtensionRef == nil {
	// 		return nil, fmt.Errorf("extensionRef not set in filter %v", obj)
	// 	}
	// 	switch obj.ExtensionRef.Kind {
	// 	case "OIDCAuth":
	// 		return buildOIDCAuthFilter(ctx, client, ns, obj.ExtensionRef, r.parent.spiffe)

	// 	case "External":
	// 		return buildExternalFilter(ctx, client, ns, obj.ExtensionRef, r.parent.spiffe)

	// 	default:
	// 		return nil, fmt.Errorf("unhandled extensionRef kind %v", obj)
	// 	}
	default:
		return nil, fmt.Errorf("unhandled filter type %v", obj)
	}
}
