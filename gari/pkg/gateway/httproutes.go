package gateway

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type httpRoutes struct {
	mutex  sync.RWMutex
	byID   map[types.NamespacedName]*httpRoute
	byHost map[string][]*httpRoute
}

func (r *httpRoutes) init() {
	r.byID = make(map[types.NamespacedName]*httpRoute)
	r.byHost = make(map[string][]*httpRoute)
}

// Should be immutable
type httpRoute struct {
	id    types.NamespacedName
	hosts []string
	obj   gatewayapi.HTTPRoute
}

func (r *httpRoutes) lookupHTTPRoute(ctx context.Context, req *http.Request) *httpRoute {
	host := req.Host

	// TODO: IPv6 wil confuse the : check
	if strings.Contains(host, ":") {
		h, _, err := net.SplitHostPort(req.Host)
		if err != nil {
			log := klog.FromContext(ctx)
			log.Info("invalid host header", "host", req.Host)
			return nil
		}
		host = h
	}

	r.mutex.RLock()
	httpRoutes := r.byHost[host]
	r.mutex.RUnlock()

	bestScore := -1
	var bestRoute *httpRoute
	for _, httpRoute := range httpRoutes {
		score := httpRoute.matches(ctx, req)
		if score > bestScore {
			bestScore = score
			bestRoute = httpRoute
		}
	}

	if bestRoute == nil {
		log := klog.FromContext(ctx)
		log.Info("no routes for host", "host", host)
	}

	return bestRoute
}

func (r *httpRoute) matches(ctx context.Context, req *http.Request) int {
	bestScore := -1

	for i := range r.obj.Spec.Rules {
		score := -1
		rule := &r.obj.Spec.Rules[i]
		if len(rule.Matches) == 0 {
			// If no matches are specified, the default is a prefix path match on “/”, which has the effect of matching every HTTP request.
			score = 1
		} else {
			allMatch := true
			for j := range rule.Matches {
				if !satisfiesMatches(ctx, &rule.Matches[j], req) {
					allMatch = false
					break
				}
			}
			if allMatch {
				score = 1
				for j := range rule.Matches {
					match := &rule.Matches[j]
					if match.Path != nil && match.Path.Value != nil {
						score += len(*match.Path.Value)
					}
					if !satisfiesMatches(ctx, &rule.Matches[j], req) {
						allMatch = false
						break
					}
				}
			}
		}
		if score > bestScore {
			bestScore = score
			// bestRule = rule
		}
	}
	return bestScore
}

func satisfiesMatches(ctx context.Context, match *gatewayapi.HTTPRouteMatch, req *http.Request) bool {
	if match.Path != nil {
		reqPath := req.URL.Path

		value := withDefault(match.Path.Value, "/")
		matchType := withDefault(match.Path.Type, gatewayapi.PathMatchPathPrefix)

		switch matchType {
		case gatewayapi.PathMatchPathPrefix:
			if !strings.HasPrefix(reqPath, value) {
				return false
			}
		case gatewayapi.PathMatchExact:
			if reqPath != value {
				return false
			}
		default:
			log := klog.FromContext(ctx)
			log.Info("unsupported path match type", "matchType", matchType)
			return false
		}
	}

	return true
}

func (r *httpRoutes) UpdateHTTPRoute(ctx context.Context, route *gatewayapi.HTTPRoute) error {
	id := types.NamespacedName{Namespace: route.GetNamespace(), Name: route.GetName()}
	return r.updateHTTPRoute(ctx, id, route)
}

func (r *httpRoutes) DeleteHTTPRoute(ctx context.Context, route *gatewayapi.HTTPRoute) error {
	id := types.NamespacedName{Namespace: route.GetNamespace(), Name: route.GetName()}
	return r.updateHTTPRoute(ctx, id, nil)
}

func (r *httpRoutes) updateHTTPRoute(ctx context.Context, id types.NamespacedName, newObj *gatewayapi.HTTPRoute) error {
	var newHTTPRoute *httpRoute
	if newObj != nil {
		var hosts []string
		for _, hostname := range newObj.Spec.Hostnames {
			hosts = append(hosts, string(hostname))
		}

		newHTTPRoute = &httpRoute{
			id:    id,
			hosts: hosts,
			obj:   *newObj.DeepCopy(),
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
