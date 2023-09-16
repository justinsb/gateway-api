package gateway

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type Filter interface {
	Handle(w http.ResponseWriter, req *http.Request) bool
}

type errorFilter struct {
	err error
}

func (f *errorFilter) Handle(w http.ResponseWriter, req *http.Request) bool {
	ctx := req.Context()

	log := klog.FromContext(ctx)

	log.Error(f.err, "filter error")
	http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
	return true
}

func buildFilter(ctx context.Context, client client.Client, ns string, obj *gatewayapi.HTTPRouteFilter) (Filter, error) {
	switch obj.Type {
	case gatewayapi.HTTPRouteFilterExtensionRef:
		if obj.ExtensionRef == nil {
			return nil, fmt.Errorf("extensionRef not set in filter %v", obj)
		}
		switch obj.ExtensionRef.Kind {
		case "OIDCAuth":
			return buildOIDCAuthFilter(ctx, client, ns, obj.ExtensionRef)

		default:
			return nil, fmt.Errorf("unhandled extensionRef kind %v", obj)
		}
	default:
		return nil, fmt.Errorf("unhandled filter type %v", obj)
	}
}
