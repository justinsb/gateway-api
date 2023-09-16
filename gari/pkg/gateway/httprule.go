package gateway

import (
	"context"
	"math"
	"net/http"
	"strings"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type httpRule struct {
	obj *gatewayapi.HTTPRouteRule

	Filters []Filter
}

func buildHTTPRule(ctx context.Context, client client.Client, ns string, obj *gatewayapi.HTTPRouteRule) httpRule {
	r := httpRule{obj: obj}
	for i := range obj.Filters {
		filter := &obj.Filters[i]
		f, err := buildFilter(ctx, client, ns, filter)
		if err != nil {
			log := klog.FromContext(ctx)
			log.Error(err, "error building filter")
			f = &errorFilter{err: err}
		}
		r.Filters = append(r.Filters, f)
	}
	return r
}

func (r *httpRule) scoreMatch(ctx context.Context, req *http.Request) int {
	score := math.MaxInt
	if len(r.obj.Matches) == 0 {
		// If no matches are specified, the default is a prefix path match on “/”, which has the effect of matching every HTTP request.
		score = 1
	} else {
		allMatch := true
		for j := range r.obj.Matches {
			if !satisfiesMatches(ctx, &r.obj.Matches[j], req) {
				allMatch = false
				break
			}
		}
		if allMatch {
			score = 1
			for j := range r.obj.Matches {
				match := &r.obj.Matches[j]
				if match.Path != nil && match.Path.Value != nil {
					score += len(*match.Path.Value)
				}
				// if !satisfiesMatches(ctx, &r.obj.Matches[j], req) {
				// 	allMatch = false
				// 	break
				// }
			}
		}
	}

	return score
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
