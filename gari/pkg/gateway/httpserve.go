package gateway

import (
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"k8s.io/klog/v2"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func (s *httpRoute) serveHTTP(w http.ResponseWriter, req *http.Request, rule *httpRule) {
	ctx := req.Context()

	backendRefs := rule.obj.BackendRefs
	if len(backendRefs) == 0 {
		log := klog.FromContext(ctx)
		log.Info("no backedRefs in rule")
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}

	for _, filter := range rule.Filters {
		if filter.Handle(w, req) {
			return
		}
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

	targetProtocol := "http"
	if s.spiffeID != "" {
		targetProtocol = "https"
	}

	director := func(req *http.Request) {
		// So backends know the original scheme
		req.Header.Set("X-Forwarded-Proto", forwardedProto)

		// targetQuery := target.RawQuery
		req.URL.Scheme = targetProtocol
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

	// TODO: Can we cache httpTransport?  by backend?
	httpTransport := &http.Transport{}

	if s.spiffeID != "" {
		// Allowed SPIFFE ID
		spiffeID := s.spiffeID
		spiffeID = strings.ReplaceAll(spiffeID, "{{namespace}}", serviceNamespace)
		spiffeID = strings.ReplaceAll(spiffeID, "{{name}}", serviceName)
		serverID := spiffeid.RequireFromString(spiffeID)

		klog.Infof("creating tlsclientconfig %q requires %q", backendHostName, spiffeID)
		// Create a `tls.Config` to allow mTLS connections, and verify that presented certificate has SPIFFE ID `spiffe://example.org/client`
		tlsClientConfig := tlsconfig.MTLSClientConfig(SPIFFE.source, SPIFFE.source, tlsconfig.AuthorizeID(serverID))

		httpTransport.TLSClientConfig = tlsClientConfig
	}

	proxy := &httputil.ReverseProxy{
		Director:  director,
		Transport: httpTransport,
	}

	proxy.ServeHTTP(w, req)
}
