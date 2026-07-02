package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"

	"github.com/kortolabs/proxy-engine/internal/compressor"
)

const (
	headerTenantID   = "X-Tenant-ID"
	headerSessionID  = "X-Session-ID"
	defaultTenantID  = "default"
	defaultSessionID = "default"
)

// ScopeResolver derives tenant/session isolation scope from incoming requests.
type ScopeResolver struct {
	TrustUpstreamGateway bool
	TrustedProxyCIDRs    []*net.IPNet
}

func (sr ScopeResolver) FromRequest(r *http.Request) compressor.Scope {
	if sr.TrustUpstreamGateway && sr.isTrustedPeer(r) {
		return scopeFromTrustedHeaders(r)
	}
	return deriveScopeFromCredentials(r)
}

func scopeFromTrustedHeaders(r *http.Request) compressor.Scope {
	tenant := strings.TrimSpace(r.Header.Get(headerTenantID))
	if tenant == "" {
		return deriveScopeFromCredentials(r)
	}

	session := strings.TrimSpace(r.Header.Get(headerSessionID))
	if session == "" {
		session = sessionFromCredentials(r)
	}

	return compressor.Scope{TenantID: tenant, SessionID: session}
}

func deriveScopeFromCredentials(r *http.Request) compressor.Scope {
	cred := extractCredential(r)
	if cred == "" {
		return compressor.Scope{TenantID: defaultTenantID, SessionID: defaultSessionID}
	}

	h := hashCredential(cred)
	scopeID := "cred:" + h
	return compressor.Scope{TenantID: scopeID, SessionID: scopeID}
}

func extractCredential(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if token != "" {
			return token
		}
	}
	if apiKey := strings.TrimSpace(r.Header.Get("x-api-key")); apiKey != "" {
		return apiKey
	}
	return ""
}

func sessionFromCredentials(r *http.Request) string {
	if cred := extractCredential(r); cred != "" {
		return hashCredential(cred)
	}
	return defaultSessionID
}

func hashCredential(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func (sr ScopeResolver) isTrustedPeer(r *http.Request) bool {
	return isTrustedPeer(r, sr.TrustedProxyCIDRs)
}

// isTrustedPeer validates the TCP socket origin only — never HTTP headers such as
// X-Forwarded-For, which untrusted clients can forge on public edge deployments.
func isTrustedPeer(r *http.Request, trustedCIDRs []*net.IPNet) bool {
	if len(trustedCIDRs) == 0 {
		return false
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	parsedIP := net.ParseIP(strings.TrimSpace(host))
	if parsedIP == nil {
		return false
	}

	for _, cidr := range trustedCIDRs {
		if cidr.Contains(parsedIP) {
			return true
		}
	}
	return false
}

func parseTrustedCIDRs(raw string) ([]*net.IPNet, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var out []*net.IPNet
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		_, network, err := net.ParseCIDR(part)
		if err != nil {
			return nil, err
		}
		out = append(out, network)
	}
	return out, nil
}
