package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"k8s.io/klog/v2"
)

type spiffe struct {
	source *workloadapi.X509Source
}

var SPIFFE spiffe

func InitSPIFFE(ctx context.Context) error {
	// TODO: Hook up to sockets?  Call init from main?
	if err := SPIFFE.Init(ctx); err != nil {
		return fmt.Errorf("initializing SPIFFE: %w", err)
	}
	return nil
}

func (s *spiffe) Init(ctx context.Context) error {
	// Worload API socket path
	const socketPath = "unix:///run/spire/sockets/agent.sock"

	klog.Infof("creating x509 source with %q", socketPath)
	// Create a `workloadapi.X509Source`, it will connect to Workload API using provided socket.
	// If socket path is not defined using `workloadapi.SourceOption`, value from environment variable `SPIFFE_ENDPOINT_SOCKET` is used.
	source, err := workloadapi.NewX509Source(ctx, workloadapi.WithClientOptions(workloadapi.WithAddr(socketPath)))
	if err != nil {
		return fmt.Errorf("unable to create X509Source: %w", err)
	}

	s.source = source
	return nil
}

func (s *spiffe) Close() error {
	var errs []error
	if s.source != nil {
		if err := s.source.Close(); err != nil {
			errs = append(errs, err)
		} else {
			s.source = nil
		}
	}
	return errors.Join(errs...)
}
