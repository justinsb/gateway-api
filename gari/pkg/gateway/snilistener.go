package gateway

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"time"

	// kinspire "github.com/justinsb/packages/kinspire/client"
	"golang.org/x/crypto/cryptobyte"

	"k8s.io/klog/v2"
)

type SNIListener struct {
	gateway *Instance
}

type SNIListenerConfig struct {
}

func (i *Instance) AddSNIListener(ctx context.Context, sniConfig SNIListenerConfig) (*SNIListener, error) {
	l := &SNIListener{
		gateway: i,
	}

	return l, nil
}

func (l *SNIListener) Start(ctx context.Context, listen string) error {
	log := klog.FromContext(ctx)
	tcpListener, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}

	go func() {
		log.Info("listening for sni", "listen", listen)
		for {
			conn, err := tcpListener.Accept()
			if err != nil {
				klog.ErrorS(err, "error accepting connection")
				continue
			}
			go l.handleConnection(ctx, conn)
		}
	}()
	return nil
}

func (l *SNIListener) handleConnection(ctx context.Context, conn net.Conn) {
	log := klog.FromContext(ctx)
	defer conn.Close()

	clientHello, err := readClientHello(conn)
	if err != nil {
		log.Error(err, "error reading client hello")
		return
	}

	extensions, err := clientHello.ParseExtensions()
	if err != nil {
		log.Error(err, "error parsing extensions")
		return
	}

	sniInfo, err := extensions.ParseServerName()
	if err != nil {
		log.Error(err, "error parsing server name")
		return
	}

	log.Info("sni info", "sniInfo", sniInfo)

	match, found := l.gateway.lookupTLSRoute(ctx, sniInfo.HostName)
	if !found {
		log.Info("no matching TLSRoute for request", "sni.host", sniInfo.HostName)
		return
	}

	if err := match.serveTLS(ctx, conn, clientHello); err != nil {
		log.Error(err, "error serving SNI connection", "sni.host", sniInfo.HostName)
	}
}

type clientHelloInfo struct {
	Header  []byte
	Message []byte

	Version            uint16
	Random             []byte
	SessionID          cryptobyte.String
	CipherSuites       cryptobyte.String
	CompressionMethods cryptobyte.String
	Extensions         cryptobyte.String
}

type parsedExtensions struct {
	ServerNameData cryptobyte.String
}

func (c *clientHelloInfo) ParseExtensions() (*parsedExtensions, error) {
	// copy the extensions
	extensions := c.Extensions

	info := &parsedExtensions{}

	for !extensions.Empty() {
		var extType uint16
		var extData cryptobyte.String
		if !extensions.ReadUint16(&extType) || !extensions.ReadUint16LengthPrefixed(&extData) {
			return nil, fmt.Errorf("malformed extensions")
		}

		if extType == 0 {
			info.ServerNameData = extData
		}
	}

	return info, nil
}

type ServerNameInfo struct {
	HostName string
}

func (p *parsedExtensions) ParseServerName() (*ServerNameInfo, error) {
	in := p.ServerNameData

	out := &ServerNameInfo{}

	var nameList cryptobyte.String
	if !in.ReadUint16LengthPrefixed(&nameList) {
		return nil, fmt.Errorf("malformed server name data (name list length)")
	}
	if nameList.Empty() {
		return nil, fmt.Errorf("empty server name data")
	}
	for !nameList.Empty() {
		var nameType uint8
		if !nameList.ReadUint8(&nameType) {
			return nil, fmt.Errorf("malformed server name data (name type)")
		}
		var nameData cryptobyte.String
		if !nameList.ReadUint16LengthPrefixed(&nameData) {
			return nil, fmt.Errorf("malformed server name data (name data)")
		}
		if nameData.Empty() {
			return nil, fmt.Errorf("empty server name data")
		}
		switch nameType {
		case 0:
			// host_name
			if out.HostName != "" {
				return nil, fmt.Errorf("multiple host names")
			}
			out.HostName = string(nameData)
		}
	}
	if !in.Empty() {
		return nil, fmt.Errorf("unexpected server name data")
	}
	return out, nil
}

// readClientHello reads the client hello from the connection
func readClientHello(clientConn net.Conn) (*clientHelloInfo, error) {
	// This approach (and using cryptobyte) is based on https://www.agwa.name/blog/post/parsing_tls_client_hello_with_cryptobyte

	out := &clientHelloInfo{}

	if err := clientConn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, err
	}

	// Read the client hello from conn
	reader := bufio.NewReader(clientConn)

	out.Header = make([]byte, 9)
	if _, err := io.ReadFull(reader, out.Header); err != nil {
		return nil, fmt.Errorf("reading client hello header: %w", err)
	}
	header := cryptobyte.String(out.Header)

	var contentType uint8
	if !header.ReadUint8(&contentType) {
		return nil, fmt.Errorf("malformed client_hello header")
	}
	if contentType != 22 {
		return nil, fmt.Errorf("expected content type 0x22, got %x", contentType)
	}

	var legacyVersion uint16
	if !header.ReadUint16(&legacyVersion) {
		return nil, fmt.Errorf("malformed client_hello header")
	}
	if legacyVersion != 0x0301 {
		return nil, fmt.Errorf("expected legacy version (0x0301), got 0x%04x", legacyVersion)
	}

	var messageLength uint16
	if !header.ReadUint16(&messageLength) {
		return nil, fmt.Errorf("malformed message length header")
	}
	if messageLength < 4 || messageLength > 1024 {
		return nil, fmt.Errorf("unexpected message length: %d", messageLength)
	}

	var messageType uint8
	if !header.ReadUint8(&messageType) {
		return nil, fmt.Errorf("malformed client_hello header")
	}
	if messageType != 1 {
		return nil, fmt.Errorf("expected client_hello (1), got %d", messageType)
	}

	var handshakeMessageLength uint32
	if !header.ReadUint24(&handshakeMessageLength) {
		return nil, fmt.Errorf("malformed client_hello header")
	}
	if handshakeMessageLength < 4 || handshakeMessageLength > 1024 {
		return nil, fmt.Errorf("unexpected client_hello message length: %d", messageLength)
	}

	out.Message = make([]byte, handshakeMessageLength)
	if _, err := io.ReadFull(reader, out.Message); err != nil {
		return nil, fmt.Errorf("reading client_hello: %w", err)
	}
	clientHello := cryptobyte.String(out.Message)

	if !clientHello.ReadUint16(&out.Version) {
		return nil, fmt.Errorf("reading client_hello version")
	}
	if !clientHello.ReadBytes(&out.Random, 32) {
		return nil, fmt.Errorf("reading client_hello random")
	}

	if !clientHello.ReadUint8LengthPrefixed(&out.SessionID) {
		return nil, fmt.Errorf("reading client_hello session ID")
	}

	if !clientHello.ReadUint16LengthPrefixed(&out.CipherSuites) {
		return nil, fmt.Errorf("reading client_hello cipher suites")
	}

	if !clientHello.ReadUint8LengthPrefixed(&out.CompressionMethods) {
		return nil, fmt.Errorf("reading client_hello compression methods")
	}

	if !clientHello.Empty() {
		if !clientHello.ReadUint16LengthPrefixed(&out.Extensions) {
			return nil, fmt.Errorf("reading client_hello extensions")
		}
	}

	if !clientHello.Empty() {
		return nil, fmt.Errorf("unexpected extra client_hello data")
	}

	if err := clientConn.SetReadDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("setting read deadline: %w", err)
	}

	return out, nil
}
