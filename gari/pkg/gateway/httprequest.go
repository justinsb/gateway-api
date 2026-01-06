package gateway

import "net/http"

// HTTPRequest is a wrapper around the processing of a single HTTP request.
type HTTPRequest struct {
	req    *http.Request
	w      http.ResponseWriter
	scheme string
}

func (r *HTTPRequest) Scheme() string {
	return r.scheme
}

func (r *HTTPRequest) Host() string {
	return r.req.Host
}

func (r *HTTPRequest) Path() string {
	return r.req.URL.Path
}
