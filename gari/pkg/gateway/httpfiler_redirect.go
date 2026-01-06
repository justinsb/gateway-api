package gateway

import (
	"context"
	"net/http"
	"net/url"

	"k8s.io/klog/v2"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
)

func buildRequestRedirectFilter(obj *gatewayapi.HTTPRequestRedirectFilter) (Filter, error) {
	return &requestRedirectFilter{obj: obj}, nil
}

type requestRedirectFilter struct {
	obj *gatewayapi.HTTPRequestRedirectFilter
}

var _ Filter = &requestRedirectFilter{}

func (f *requestRedirectFilter) Handle(ctx context.Context, httpRequest *HTTPRequest) bool {
	redirect := &url.URL{}
	if f.obj.Scheme != nil {
		redirect.Scheme = ValueOf(f.obj.Scheme)
	} else {
		redirect.Scheme = httpRequest.Scheme()
	}
	if f.obj.Hostname != nil {
		redirect.Host = string(ValueOf(f.obj.Hostname))
	} else {
		redirect.Host = httpRequest.Host()
	}
	redirect.Path = httpRequest.Path()
	if f.obj.Path != nil {
		// TODO: Handle path modifier
	}

	statusCode := http.StatusFound
	if f.obj.StatusCode != nil {
		statusCode = ValueOf(f.obj.StatusCode)
	}
	klog.Infof("redirecting to %v with status code %v", redirect.String(), statusCode)
	klog.Infof("rule is %+v", f.obj)

	http.Redirect(httpRequest.w, httpRequest.req, redirect.String(), statusCode)
	return true // Handled by this filter
}

func ValueOf[T any](v *T) T {
	if v == nil {
		return zeroValue[T]()
	}
	return *v
}

func zeroValue[T any]() T {
	var v T
	return v
}
