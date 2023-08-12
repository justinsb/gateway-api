package gateway

import (
	"context"
	"net/http"

	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type Instance struct {
	httpRoutes httpRoutes
}

func New() (*Instance, error) {
	i := &Instance{}
	i.httpRoutes.init()
	return i, nil
}

func (i *Instance) lookupHTTPRoute(ctx context.Context, req *http.Request) *httpRoute {
	return i.httpRoutes.lookupHTTPRoute(ctx, req)
}

func (i *Instance) UpdateHTTPRoute(ctx context.Context, route *gatewayapi.HTTPRoute) error {
	return i.httpRoutes.UpdateHTTPRoute(ctx, route)
}

func (i *Instance) DeleteHTTPRoute(ctx context.Context, route *gatewayapi.HTTPRoute) error {
	return i.httpRoutes.DeleteHTTPRoute(ctx, route)
}
