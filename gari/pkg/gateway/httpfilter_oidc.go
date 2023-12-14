package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/gateway-api/gari/apis/v1alpha1"
	"sigs.k8s.io/gateway-api/gari/pkg/oidc"

	kinspire "github.com/justinsb/packages/kinspire/client"
)

type oidcAuthFilter struct {
	loginURL      string
	authProviders []*oidc.Provider
	spiffe        *kinspire.SPIFFESource
}

var _ Filter = &oidcAuthFilter{}

//+kubebuilder:rbac:groups=gari.gateway.networking.x-k8s.io,resources=oidcauths,verbs=get;list;watch

func buildOIDCAuthFilter(ctx context.Context, client client.Client, ns string, ref *gatewayapi.LocalObjectReference, spiffe *kinspire.SPIFFESource) (*oidcAuthFilter, error) {
	obj := &v1alpha1.OIDCAuth{}
	id := types.NamespacedName{
		Name:      string(ref.Name),
		Namespace: ns,
	}
	if ref.Name == "" {
		return nil, fmt.Errorf("name not set")
	}
	if err := client.Get(ctx, id, obj); err != nil {
		return nil, fmt.Errorf("getting oidcauth object: %w", err)
	}

	var authProviders []*oidc.Provider
	authProvider, err := oidc.NewProvider(obj.Spec.Issuer, obj.Spec.Audience)
	if err != nil {
		return nil, fmt.Errorf("error building provider: %w", err)
	}
	authProviders = append(authProviders, authProvider)

	return &oidcAuthFilter{
		loginURL:      obj.Spec.LoginURL,
		authProviders: authProviders,
		spiffe:        spiffe,
	}, nil
}

func (f *oidcAuthFilter) Handle(w http.ResponseWriter, req *http.Request) bool {
	ctx := req.Context()

	log := klog.FromContext(ctx)

	jwt := ""
	cookie, err := req.Cookie("auth-token")
	if err != nil {
		if err == http.ErrNoCookie {
			jwt = ""
		} else {
			log.Error(err, "error getting auth cookie")
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
			return true
		}
	} else {
		jwt = cookie.Value
	}

	if jwt != "" {
		auth, err := oidc.VerifyToken(ctx, jwt, f.authProviders)
		if err != nil {
			// This means that we had an internal error, not that the token was bad
			log.Error(err, "error verifying token")
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
			return true
		}
		if auth == nil {
			log.Info("rejecting invalid sso token")
			jwt = ""
		}
	}

	if jwt == "" {
		loginURL, err := url.Parse(f.loginURL)
		if err != nil {
			log.Error(err, "error parsing login url", "url", f.loginURL)
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
			return true
		}
		q := loginURL.Query()
		redirectURL := *req.URL
		redirectURL.Scheme = "https" // TODO
		redirectURL.Host = req.Host
		q.Add("redirect", redirectURL.String())
		loginURL.RawQuery = q.Encode()
		http.Redirect(w, req, loginURL.String(), http.StatusSeeOther)
		return true
	}

	return false
}
