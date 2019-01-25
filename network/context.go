package network

import (
	"context"
)

type (
	signMessageCtxKeyType string
	requestIPCtxKeyType   string
)

const (
	signMessageCtxKey signMessageCtxKeyType = "signMessage"
	requestIPCtxKey   requestIPCtxKeyType   = "requestIP"
)

// WithSignMessage sets whether the request should be signed
func WithSignMessage(ctx context.Context, sign bool) context.Context {
	return context.WithValue(ctx, signMessageCtxKey, sign)
}

// GetSignMessage returns whether the request should be signed
func GetSignMessage(ctx context.Context) bool {
	sign, ok := ctx.Value(signMessageCtxKey).(bool)
	if !ok {
		return false
	}
	return sign
}

func WithUDPRequestIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, requestIPCtxKey, ip)
}

func GetUDPRequestIP(ctx context.Context) (string, bool) {
	ip, ok := ctx.Value(requestIPCtxKey).(string)
	if !ok {
		return "", false
	}
	return ip, true
}
