package utils

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateProxyTargetURL guards the generic API proxy (/api/proxy/*) against
// SSRF. The target is taken from the caller-controlled X-API-Base-URL header
// and forwarded to the agent, so a leaked relay secret (or any trusted internal
// caller) could otherwise pivot the proxy into internal/loopback/metadata
// endpoints (CWE-918).
//
// Always rejected:
//   - schemes other than http/https
//   - the localhost / cloud-metadata hostnames
//   - IP literals that are loopback, link-local (which includes the
//     169.254.169.254 cloud metadata address), or the unspecified address
//
// When blockPrivate is true, RFC1918 / ULA private ranges (which include the
// IMDSv6 fd00:ec2::254 address) are rejected as well.
//
// Only IP-literal hosts are range-checked: the relay runs in a different
// network than the agent that ultimately makes the request, so it cannot (and
// must not) resolve customer-internal hostnames. Full per-IP validation belongs
// on the agent side as defense in depth.
func ValidateProxyTargetURL(rawURL string, blockPrivate bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid target URL: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		// allowed
	default:
		return fmt.Errorf("unsupported target URL scheme %q (only http/https allowed)", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("target URL has no host")
	}

	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") ||
		lower == "metadata" || lower == "metadata.google.internal" {
		return fmt.Errorf("target host %q is not an allowed proxy target", host)
	}

	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("target IP %s is in a blocked range (loopback/link-local/metadata)", host)
		}
		if blockPrivate && ip.IsPrivate() {
			return fmt.Errorf("target IP %s is in a private range (blocked by RELAY_PROXY_BLOCK_PRIVATE_TARGETS)", host)
		}
	}

	return nil
}
