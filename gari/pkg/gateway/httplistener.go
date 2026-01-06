package gateway

import (
	"context"
	"net"
	"net/http"
	"time"

	"k8s.io/klog/v2"
)

type HTTPListener struct {
	gateway    *Instance
	httpServer *http.Server
}

func (i *Instance) AddHTTPListener(ctx context.Context) (*HTTPListener, error) {
	l := &HTTPListener{
		gateway: i,
	}

	l.httpServer = &http.Server{
		ReadTimeout:       1 * time.Second,
		WriteTimeout:      1 * time.Second,
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		// TLSConfig:         tlsConfig,
		Handler: l,
	}

	return l, nil
}

func (l *HTTPListener) Start(ctx context.Context, listen string) error {
	log := klog.FromContext(ctx)
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	go func() {
		log.Info("listening for http", "listen", listen)
		if err := l.httpServer.Serve(ln); err != nil {
			klog.ErrorS(err, "error from http server")
		}
	}()
	return nil
}

func (l *HTTPListener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	{
		log := klog.FromContext(ctx)
		log.Info("serving HTTP request", "host", r.Host, "url", r.URL)
	}

	match, found := l.gateway.lookupHTTPRoute(ctx, r)
	if !found {
		log := klog.FromContext(ctx)
		log.Info("no matching HTTPRoute for request", "url", r.URL)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}

	httpRequest := &HTTPRequest{
		req: r,
		w:   w,
	}

	if httpRequest.req.TLS != nil {
		httpRequest.scheme = "https"
	} else {
		httpRequest.scheme = "http"
	}

	match.route.serveHTTP(ctx, httpRequest, match.rule)
}
