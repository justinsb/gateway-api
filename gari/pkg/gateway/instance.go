package gateway

import (
	"context"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type Instance struct {
	httpRoutes httpRoutes
}

func New(spiffeID string) (*Instance, error) {
	i := &Instance{}
	i.httpRoutes.spiffeID = spiffeID
	i.httpRoutes.init()
	return i, nil
}

func (i *Instance) lookupHTTPRoute(ctx context.Context, req *http.Request) (routeMatch, bool) {
	return i.httpRoutes.lookupHTTPRoute(ctx, req)
}

func (i *Instance) UpdateHTTPRoute(ctx context.Context, client client.Client, route *gatewayapi.HTTPRoute) error {
	return i.httpRoutes.UpdateHTTPRoute(ctx, client, route)
}

func (i *Instance) DeleteHTTPRoute(ctx context.Context, client client.Client, route *gatewayapi.HTTPRoute) error {
	return i.httpRoutes.DeleteHTTPRoute(ctx, client, route)
}
