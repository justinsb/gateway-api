package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"k8s.io/klog/v2"
)

func VerifyToken(ctx context.Context, token string, providers []*Provider) (*AuthInfo, error) {
	klog.Infof("token is %q", token)
	token = strings.TrimPrefix(token, "Bearer ")
	components := strings.Split(token, ".")
	if len(components) != 3 {
		klog.Infof("jwt token did not have expected number of components")
		return nil, nil
	}

	parsed := &Token{}
	parsed.Raw = token

	for i, s := range components {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			klog.Infof("jwt token had invalid base64")
			return nil, nil
		}
		switch i {
		case 0:
			if err := json.Unmarshal(b, &parsed.Header); err != nil {
				klog.Infof("error parsing jwt header: %v", err)
				return nil, nil
			}
		case 1:
			if err := json.Unmarshal(b, &parsed.Payload); err != nil {
				klog.Infof("error parsing jwt payload: %v", err)
				return nil, nil
			}
		}
	}

	klog.Infof("decoded JWT token header=%+v payload=%+v", parsed.Header, parsed.Payload)

	var errs []error
	for _, provider := range providers {
		auth, err := provider.VerifyToken(parsed)
		if err != nil {
			errs = append(errs, err)
		} else if auth != nil {
			return auth, nil
		}
	}

	return nil, errors.Join(errs...)
}
