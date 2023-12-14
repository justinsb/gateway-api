package gateway

import (
	"context"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"

	kinspire "github.com/justinsb/packages/kinspire/client"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type httpRoutes struct {
	spiffeID string
	spiffe   *kinspire.SPIFFESource
	mutex    sync.RWMutex
	byID     map[types.NamespacedName]*httpRoute
	byHost   map[string][]*httpRoute
}

func (r *httpRoutes) init() {
	r.byID = make(map[types.NamespacedName]*httpRoute)
	r.byHost = make(map[string][]*httpRoute)
}

// Should be immutable
type httpRoute struct {
	id       types.NamespacedName
	hosts    []string
	spiffeID string
	spiffe   *kinspire.SPIFFESource
	obj      gatewayapi.HTTPRoute
	rules    []httpRule
}

func (r *httpRoutes) lookupHTTPRoute(ctx context.Context, req *http.Request) (routeMatch, bool) {
	host := req.Host

	// TODO: IPv6 wil confuse the : check
	if strings.Contains(host, ":") {
		h, _, err := net.SplitHostPort(req.Host)
		if err != nil {
			log := klog.FromContext(ctx)
			log.Info("invalid host header", "host", req.Host)
			return routeMatch{}, false
		}
		host = h
	}

	r.mutex.RLock()
	httpRoutes := r.byHost[host]
	r.mutex.RUnlock()

	var bestMatch routeMatch
	bestMatch.score = math.MinInt
	for _, httpRoute := range httpRoutes {
		routeMatch, found := httpRoute.matches(ctx, req)
		if found && routeMatch.score > bestMatch.score {
			bestMatch = routeMatch
		}
	}

	if bestMatch.score == math.MinInt {
		log := klog.FromContext(ctx)
		log.Info("no routes for host", "host", host)
		return routeMatch{}, false
	}

	return bestMatch, true
}

type routeMatch struct {
	score int
	route *httpRoute
	rule  *httpRule
}

func (r *httpRoute) matches(ctx context.Context, req *http.Request) (routeMatch, bool) {
	var bestMatch routeMatch
	bestMatch.score = math.MinInt

	for i := range r.rules {
		rule := &r.rules[i]
		score := rule.scoreMatch(ctx, req)
		if score > bestMatch.score {
			bestMatch = routeMatch{
				score: score,
				route: r,
				rule:  rule,
			}
		}
	}

	if bestMatch.score == math.MinInt {
		return routeMatch{}, false
	}
	return bestMatch, true
}

func (r *httpRoutes) UpdateHTTPRoute(ctx context.Context, client client.Client, route *gatewayapi.HTTPRoute) error {
	id := types.NamespacedName{Namespace: route.GetNamespace(), Name: route.GetName()}
	return r.updateHTTPRoute(ctx, client, id, route)
}

func (r *httpRoutes) DeleteHTTPRoute(ctx context.Context, client client.Client, route *gatewayapi.HTTPRoute) error {
	id := types.NamespacedName{Namespace: route.GetNamespace(), Name: route.GetName()}
	return r.updateHTTPRoute(ctx, client, id, nil)
}

func (r *httpRoutes) updateHTTPRoute(ctx context.Context, client client.Client, id types.NamespacedName, newObj *gatewayapi.HTTPRoute) error {
	var newHTTPRoute *httpRoute
	if newObj != nil {
		var hosts []string
		for _, hostname := range newObj.Spec.Hostnames {
			hosts = append(hosts, string(hostname))
		}

		newHTTPRoute = &httpRoute{
			id:       id,
			hosts:    hosts,
			spiffeID: r.spiffeID,
			spiffe:   r.spiffe,
			obj:      *newObj.DeepCopy(),
		}

		for i := range newObj.Spec.Rules {
			rule := &newObj.Spec.Rules[i]
			hr := r.buildHTTPRule(ctx, client, id.Namespace, rule)
			newHTTPRoute.rules = append(newHTTPRoute.rules, hr)
		}
	}

	r.mutex.Lock()

	oldHTTPRoute := r.byID[id]
	if newHTTPRoute == nil {
		delete(r.byID, id)
	} else {
		r.byID[id] = newHTTPRoute
	}

	if oldHTTPRoute != nil {
		for _, host := range oldHTTPRoute.hosts {
			keepRoutes := filter(r.byHost[host], func(r *httpRoute) bool {
				return r.id != id
			})

			if len(keepRoutes) == 0 {
				delete(r.byHost, host)
			} else {
				r.byHost[host] = keepRoutes
			}
		}
	}

	if newHTTPRoute != nil {
		for _, host := range newHTTPRoute.hosts {
			r.byHost[host] = append(r.byHost[host], newHTTPRoute)
		}
	}

	r.mutex.Unlock()

	return nil
}
