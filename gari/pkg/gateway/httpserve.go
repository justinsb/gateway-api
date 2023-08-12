package gateway

import (
	"net/http"
	"net/http/httputil"
	"strconv"

	"k8s.io/klog/v2"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func (s *httpRoute) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// TODO: Which rule matched?
	rule := s.obj.Spec.Rules[0]

	backendRefs := rule.BackendRefs
	if len(backendRefs) == 0 {
		log := klog.FromContext(ctx)
		log.Info("no backedRefs in rule")
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}

	// TODO: Better load balancing etc
	backendRef := backendRefs[0]

	// TODO: Go direct to endpoints?
	serviceName := string(backendRef.Name)
	serviceNamespace := "" // TODO: backendRef.Namespace
	if serviceNamespace == "" {
		serviceNamespace = s.id.Namespace
	}
	backendHostName := serviceName + "." + serviceNamespace
	backendPort := gatewayapi.PortNumber(0)
	if backendRef.Port != nil {
		backendPort = *(backendRef.Port)
	}
	if backendPort == 0 {
		log := klog.FromContext(ctx)
		log.Info("cannot infer backendRef port")
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}

	// This feels "wrong", but we don't have the scheme in e.g. the URL
	forwardedProto := "http"
	if req.TLS != nil {
		forwardedProto = "https"
	}

	director := func(req *http.Request) {
		// So backends know the original scheme
		req.Header.Set("X-Forwarded-Proto", forwardedProto)

		// targetQuery := target.RawQuery
		req.URL.Scheme = "http"
		req.URL.Host = backendHostName + ":" + strconv.Itoa(int(backendPort))
		// req.URL.Path, req.URL.RawPath = joinURLPath(target, req.URL)
		// if targetQuery == "" || req.URL.RawQuery == "" {
		// 	req.URL.RawQuery = targetQuery + req.URL.RawQuery
		// } else {
		// 	req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		// }
		// log := klog.FromContext(ctx)
		// log.Info("rewrote url", "url", req.URL)
	}

	proxy := &httputil.ReverseProxy{
		Director: director,
	}

	proxy.ServeHTTP(w, req)
}
