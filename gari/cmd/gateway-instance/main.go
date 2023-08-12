package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
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

func run(ctx context.Context) error {
	listen := ":8080"
	flag.Parse()

	gw, err := gateway.New()
	if err != nil {
		return err
	}

	if listener, err := gw.AddHTTPListener(ctx); err != nil {
		return err
	} else if err := listener.Start(ctx, listen); err != nil {
		return err
	}

	op := commonoperator.Operator{}
	op.RegisterSchema(gatewayv1beta1.AddToScheme)
	op.RegisterReconciler(&controllers.HTTPRouteController{Gateway: gw})
	op.RunMain()
	return nil
}
