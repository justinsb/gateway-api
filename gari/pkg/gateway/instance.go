package gateway

import (
	"context"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayexperimental "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type Instance struct {
	httpRoutes httpRoutes
	tlsRoutes  tlsRoutes
	// spiffe     *kinspire.SPIFFESource
}

// func New(spiffe *kinspire.SPIFFESource, spiffeID string) (*Instance, error) {
func New() (*Instance, error) {
	i := &Instance{}
	// i.spiffe = spiffe
	// i.httpRoutes.spiffeID = spiffeID
	// i.httpRoutes.spiffe = spiffe
	i.httpRoutes.init()
	i.tlsRoutes.init()
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

func (i *Instance) lookupTLSRoute(ctx context.Context, host string) (tlsRouteMatch, bool) {
	return i.tlsRoutes.lookupTLSRoute(ctx, host)
}

func (i *Instance) UpdateTLSRoute(ctx context.Context, client client.Client, route *gatewayexperimental.TLSRoute) error {
	return i.tlsRoutes.UpdateTLSRoute(ctx, client, route)
}

func (i *Instance) DeleteTLSRoute(ctx context.Context, client client.Client, route *gatewayexperimental.TLSRoute) error {
	return i.tlsRoutes.DeleteTLSRoute(ctx, client, route)
}
