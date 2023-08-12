package gateway

import (
	"context"
	"crypto/tls"
	"net"
	"path/filepath"

	"k8s.io/klog/v2"
)

type HTTPSListener struct {
	gateway      *Instance
	http         *HTTPListener
	tlsConfig    *tls.Config
	certificates []*certificate
}

type TLSConfig struct {
	Host string
	Dir  string
}

func (i *Instance) AddHTTPSListener(ctx context.Context, http *HTTPListener, tlsOptions []TLSConfig) (*HTTPSListener, error) {
	var certificates []*certificate
	for _, tlsOption := range tlsOptions {
		cert, err := newCertificate(tlsOption)
		if err != nil {
			return nil, err
		}
		certificates = append(certificates, cert)
	}
	l := &HTTPSListener{
		gateway:      i,
		http:         http,
		certificates: certificates,
	}
	l.tlsConfig = &tls.Config{
		GetCertificate: l.getCertificate,
		// MinVersion:               tls.VersionTLS13,
		// PreferServerCipherSuites: true,
	}

	return l, nil
}

func (l *HTTPSListener) Start(ctx context.Context, listen string) error {
	log := klog.FromContext(ctx)
	tcpListener, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	tlsConfig := l.tlsConfig
	tlsListener := tls.NewListener(tcpListener, tlsConfig)

	go func() {
		log.Info("listening for https", "listen", listen)
		if err := l.http.httpServer.Serve(tlsListener); err != nil {
			klog.ErrorS(err, "error from https server")
		}
	}()
	return nil
}

func (l *HTTPSListener) getCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	serverName := clientHello.ServerName

	for _, certificate := range l.certificates {
		if certificate.matches(serverName) {
			// klog.InfoS("found matching certificate", "serverName", serverName, "certificate", certificate.certificate)
			return &certificate.certificate, nil
		}
	}
	klog.InfoS("no certificate found for https", "serverName", serverName)

	return nil, nil
}

type certificate struct {
	host        string
	certificate tls.Certificate
}

func newCertificate(opt TLSConfig) (*certificate, error) {
	certFile := filepath.Join(opt.Dir, "tls.crt")
	keyFile := filepath.Join(opt.Dir, "tls.key")
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	c := &certificate{
		host:        opt.Host,
		certificate: cert,
	}
	return c, nil
}

func (c *certificate) matches(hostname string) bool {
	return c.host == hostname
}
