package oidc

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

type Provider struct {
	baseURL  url.URL
	audience string

	oidcConfiguration *oidcConfiguration
	parsedKeySet      *keySet
}

func NewProvider(baseURL string, audience string) (*Provider, error) {
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing url %q: %w", baseURL, err)
	}

	return &Provider{baseURL: *parsedBaseURL, audience: audience}, nil
}

type Token struct {
	Raw string

	Header  JWTHeader
	Payload JWTPayload
}

type JWTHeader struct {
	Algorithm string `json:"alg,omitempty"`
	KeyType   string `json:"typ,omitempty"`
	KeyID     string `json:"kid,omitempty"`
}

type JWTPayload struct {
	Issuer     string `json:"iss,omitempty"`
	Scope      string `json:"scope,omitempty"`
	Audience   string `json:"aud,omitempty"`
	Expiration int64  `json:"exp,omitempty"`
	IssuedAt   int64  `json:"iat,omitempty"`
	Subject    string `json:"sub,omitempty"`
}

type AuthInfo struct {
	Issuer  string
	Subject string
}

func (p *Provider) VerifyToken(token *Token) (*AuthInfo, error) {
	if token.Header.KeyID == "" || token.Header.KeyType != "JWT" {
		klog.Infof("keyid not set / not jwt")
		return nil, nil
	}

	if p.oidcConfiguration == nil {
		// TODO: Invalidation ?
		ctx := context.Background()
		oidcConfiguration, err := p.getProviderConfiguration(ctx)
		if err != nil {
			return nil, fmt.Errorf("contacting auth provider for oidc configuration: %w", err)
		}
		p.oidcConfiguration = oidcConfiguration
	}

	if token.Payload.Issuer != p.oidcConfiguration.Issuer {
		// Not issued by this provider
		klog.Infof("issuer does not match")
		return nil, nil
	}

	if token.Payload.Audience != p.audience {
		// Issued by this provider, but not intended for us
		klog.Info("audience does not match")
		return nil, nil
	}

	timeNow := time.Now().Unix()
	if timeNow > token.Payload.Expiration {
		// token has expired
		klog.Infof("token has expired")
		return nil, nil
	}
	if timeNow < token.Payload.IssuedAt {
		// token is not yet valid
		klog.Infof("token issuedAt time not yet reached")
		return nil, nil
	}

	if p.parsedKeySet == nil {
		// TODO: Invalidation
		ctx := context.Background()
		keySet, err := p.getKeyset(ctx, p.oidcConfiguration)
		if err != nil {
			return nil, fmt.Errorf("contacting auth provider for jwks: %w", err)
		}
		p.parsedKeySet = keySet
	}

	keyset := p.parsedKeySet
	key := keyset.Keys[token.Header.KeyID]
	if key == nil {
		klog.Infof("no key with keyid %q", token.Header.KeyID)
		return nil, nil
	}

	switch token.Header.Algorithm {
	case "RS256":
		rsaPublicKey, ok := key.PublicKey.(*rsa.PublicKey)
		if !ok {
			klog.Infof("token algorithm was %q, but key type was %T", token.Header.Algorithm, key.PublicKey)
			return nil, nil
		}
		if !p.verifyRSAToken(token, rsaPublicKey) {
			klog.Infof("token signature was not valid")
			return nil, nil
		}

	default:
		// Note: don't support every algorithm, e.g. plaintext
		klog.Infof("token algorithm %q not supported", token.Header.Algorithm)
		return nil, nil
	}

	return &AuthInfo{Subject: token.Payload.Subject, Issuer: token.Payload.Issuer}, nil
}

func (p *Provider) verifyRSAToken(token *Token, key *rsa.PublicKey) bool {
	components := strings.Split(token.Raw, ".")
	if len(components) != 3 {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(components[2])
	if err != nil {
		return false
	}

	data := []byte(components[0] + "." + components[1])
	hashed := sha256.Sum256(data)

	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed[:], sig); err != nil {
		return false
	}

	return true
}

type oidcConfiguration struct {
	Issuer  string `json:"issuer"`
	JwksURI string `json:"jwks_uri"`
}

func (p *Provider) getURL(ctx context.Context, u string) ([]byte, error) {
	httpClient := http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	response, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doing http request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected http response status %q", response.Status)
	}

	b, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("reading http response body: %w", err)
	}

	return b, nil
}

func (p *Provider) getProviderConfiguration(ctx context.Context) (*oidcConfiguration, error) {
	u := p.baseURL.JoinPath(".well-known", "openid-configuration")
	b, err := p.getURL(ctx, u.String())
	if err != nil {
		return nil, err
	}

	config := &oidcConfiguration{}
	if err := json.Unmarshal(b, config); err != nil {
		return nil, fmt.Errorf("parsing http response: %w", err)
	}

	return config, nil
}

func (p *Provider) getKeyset(ctx context.Context, config *oidcConfiguration) (*keySet, error) {
	if config.JwksURI == "" {
		return nil, fmt.Errorf("jwks_uri is not valid")
	}

	b, err := p.getURL(ctx, config.JwksURI)
	if err != nil {
		return nil, err
	}

	klog.Infof("fetched jwks keyset data %v", string(b))

	data := &oidcJSONWebKeySet{}
	if err := json.Unmarshal(b, data); err != nil {
		return nil, fmt.Errorf("parsing http response: %w", err)
	}

	keySet, err := parseKeyset(data)
	if err != nil {
		return nil, fmt.Errorf("parsing keyset: %w", err)
	}

	return keySet, nil
}

type oidcJSONWebKeySet struct {
	Keys []oidcJSONWebKey `json:"keys,omitempty"`
}

type keySet struct {
	Keys map[string]*key
}

func parseKeyset(data *oidcJSONWebKeySet) (*keySet, error) {
	out := &keySet{Keys: make(map[string]*key)}
	for _, key := range data.Keys {
		if key.KeyID == "" {
			return nil, fmt.Errorf("missing key id parameter")
		}
		k, err := parseKey(&key)
		if err != nil {
			return nil, err
		}
		out.Keys[key.KeyID] = k
	}
	return out, nil
}

type oidcJSONWebKey struct {
	KeyType string `json:"kty,omitempty"`
	KeyID   string `json:"kid,omitempty"`
	N       string `json:"n,omitempty"`
	E       string `json:"e,omitempty"`
}

type key struct {
	PublicKey crypto.PublicKey
}

func parseKey(data *oidcJSONWebKey) (*key, error) {
	switch data.KeyType {
	case "RSA":
		k, err := parseRSAKey(data)
		if err != nil {
			return nil, err
		}
		return &key{PublicKey: k}, nil
	default:
		return nil, fmt.Errorf("key type %q not handled", data.KeyType)
	}
}

func parseRSAKey(data *oidcJSONWebKey) (*rsa.PublicKey, error) {
	n, err := decodeBigInt(data.N)
	if err != nil {
		return nil, err
	}
	e, err := decodeBigInt(data.E)
	if err != nil {
		return nil, err
	}

	if !e.IsInt64() {
		return nil, fmt.Errorf("invalid E value - not int64")
	}

	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

func decodeBigInt(s string) (*big.Int, error) {
	if s == "" {
		return nil, fmt.Errorf("required parameter not set")
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("parameter is not valid base64")
	}
	n := &big.Int{}
	n.SetBytes(b)
	return n, nil
}
