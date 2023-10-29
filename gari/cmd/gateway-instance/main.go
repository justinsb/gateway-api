package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"k8s.io/klog/v2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/gateway-api/gari/apis/v1alpha1"
	"sigs.k8s.io/gateway-api/gari/pkg/commonoperator"
	"sigs.k8s.io/gateway-api/gari/pkg/controllers"
	"sigs.k8s.io/gateway-api/gari/pkg/gateway"
)

func main() {
	err := run(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

type tlsFlag struct {
	Host string
	Dir  string
}

func (f *tlsFlag) String() string {
	return fmt.Sprintf("Host=%s,Dir=%s", f.Host, f.Dir)
}

type tlsFlags []tlsFlag

func (f *tlsFlags) String() string {
	var s strings.Builder
	for _, o := range *f {
		s.WriteString(o.String())
		s.WriteString(",")
	}
	return s.String()
}

func (f *tlsFlags) Set(value string) error {
	tokens := strings.Split(value, ":")
	if len(tokens) != 2 {
		return fmt.Errorf("unexpected --tls value %q", value)
	}
	*f = append(*f, tlsFlag{
		Host: tokens[0],
		Dir:  tokens[1],
	})
	return nil
}

func run(ctx context.Context) error {
	log := klog.FromContext(ctx)

	httpListen := ":8080"
	httpsListen := ":8443"

	var spiffeID string
	flag.StringVar(&spiffeID, "spiffe", spiffeID, "spiffe ID for backend communication")
	var tlsFlags tlsFlags
	flag.Var(&tlsFlags, "tls", "tls configuration")

	flag.Parse()

	if spiffeID != "" {
		if err := gateway.InitSPIFFE(ctx); err != nil {
			return err
		}
	}

	gw, err := gateway.New(spiffeID)
	if err != nil {
		return err
	}

	httpListener, err := gw.AddHTTPListener(ctx)
	if err != nil {
		return err
	}

	if err := httpListener.Start(ctx, httpListen); err != nil {
		return err
	}

	log.Info("tls configuration", "tlsFlags", tlsFlags)
	if len(tlsFlags) != 0 {
		var tlsOptions []gateway.TLSConfig
		for _, tlsFlag := range tlsFlags {
			tlsOptions = append(tlsOptions, gateway.TLSConfig{
				Dir:  tlsFlag.Dir,
				Host: tlsFlag.Host,
			})
		}
		if listener, err := gw.AddHTTPSListener(ctx, httpListener, tlsOptions); err != nil {
			return err
		} else if err := listener.Start(ctx, httpsListen); err != nil {
			return err
		}
	}

	op := commonoperator.Operator{}
	op.RegisterSchema(gatewayv1beta1.AddToScheme)
	op.RegisterSchema(v1alpha1.AddToScheme)
	op.RegisterReconciler(&controllers.HTTPRouteController{Gateway: gw})
	op.RunMain()
	return nil
}
