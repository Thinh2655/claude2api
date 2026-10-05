package core

import (
	"claude2api/config"
	"claude2api/logger"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/imroc/req/v3"
)

const claudeBaseURL = "https://claude.ai"
const localTokenCookie = "claude2api_session"

// GatewayClient builds a browser-fingerprinted client for proxying requests
func GatewayClient(sessionKey string, timeout time.Duration) *req.Client {
	client := req.C().ImpersonateChrome().SetTimeout(timeout)
	client.Transport.SetResponseHeaderTimeout(time.Second * 120)
	if config.ConfigInstance.Proxy != "" {
		client.SetProxyURL(config.ConfigInstance.Proxy)
	}
	return client
}

// gatewayHandler reverse-proxies claude.ai onto the local port so the real
// web UI works from the browser against localhost, authenticated with one of
// the configured sessionKeys.
type gatewayHandler struct {
	proxy *httputil.ReverseProxy
}

func (h *gatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.proxy.ServeHTTP(w, r)
}

func NewGatewayHandler() http.Handler {
	target, _ := url.Parse(claudeBaseURL)
	h := &gatewayHandler{
		proxy: httputil.NewSingleHostReverseProxy(target),
	}
	// Use the browser-fingerprinted transport from imroc/req so Cloudflare
	// sees a Chrome TLS/HTTP2 signature instead of Go's default client
	gwClient := req.C().ImpersonateChrome().SetTimeout(5 * time.Minute)
	if config.ConfigInstance.Proxy != "" {
		gwClient.SetProxyURL(config.ConfigInstance.Proxy)
	}
	h.proxy.Transport = gwClient.Transport
	origDirector := h.proxy.Director
	h.proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = "claude.ai"
		gatewayRewriteRequest(req)
		if strings.Contains(req.URL.Path, "/completion") {
			logger.Info(fmt.Sprintf("gateway completion %s — headers: %v", req.URL.Path, req.Header))
		}
	}
	h.proxy.ModifyResponse = gatewayModifyResponse
	h.proxy.FlushInterval = 100 * time.Millisecond // stream SSE promptly
	h.proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error(fmt.Sprintf("gateway error for %s %s: %v", r.Method, r.URL.Path, err))
		w.WriteHeader(http.StatusBadGateway)
	}
	return h
}

// pickSession picks the session for gateway upstream requests. Order:
// (1) the explicit GATEWAY_KEY the user selected (always honored, cookie or not);
// (2) the first session that carries a full browser cookie file, so the
// proxied web UI reaches claude.ai with the same cookie state the real browser
// uses when no account is pinned;
// (3) the first pool session.
func pickSession() config.SessionInfo {
	sessions := config.ConfigInstance.Sessions
	if len(sessions) == 0 {
		return config.SessionInfo{}
	}
	usable := func(s config.SessionInfo) bool {
		if s.Disabled {
			return false
		}
		if _, _, _, limited := config.SessionLimited(s.SessionKey); limited {
			return false
		}
		return true
	}
	if gw := config.ConfigInstance.GetGatewayKey(); gw != "" {
		for i := range sessions {
			if sessions[i].SessionKey == gw {
				if usable(sessions[i]) {
					return sessions[i]
				}
				break
			}
		}
	}
	for i := range sessions {
		if sessions[i].ExtraCookie != "" && usable(sessions[i]) {
			return sessions[i]
		}
	}
	for i := range sessions {
		if usable(sessions[i]) {
			return sessions[i]
		}
	}
	return sessions[0]
}

// chromeCommonHeaders are what ImpersonateChrome would set. The gateway
// transport carries the Chrome TLS fingerprint but NOT the header set (that
// lives in req.Client), and Cloudflare rejects claude.ai requests that have
// the TLS signature without matching browser headers, so we apply them here.
var chromeCommonHeaders = map[string]string{
	"pragma":                    "no-cache",
	"cache-control":             "no-cache",
	"sec-ch-ua":                 `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`,
	"sec-ch-ua-mobile":          "?0",
	"sec-ch-ua-platform":        `"Windows"`,
	"user-agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
	"accept-language":           "en-US,en;q=0.9",
}

// gatewayRewriteRequest strips hop-by-hop/browser headers, injects browser
// headers + the sessionKey cookie before forwarding to claude.ai.
func gatewayRewriteRequest(req *http.Request) {
	dropHeaders := []string{
		"x-forwarded-for", "x-forwarded-host", "x-forwarded-proto",
		"x-real-ip", "cf-connecting-ip", "cf-ray", "cf-visitor",
		"forwarded", "via",
	}
	for _, name := range dropHeaders {
		req.Header.Del(name)
	}

	for k, v := range chromeCommonHeaders {
		req.Header.Set(k, v)
	}
	if req.Method == http.MethodPost || req.Method == http.MethodPut ||
		req.Method == http.MethodPatch || req.Method == http.MethodDelete {
		req.Header.Set("sec-fetch-site", "same-origin")
		req.Header.Set("sec-fetch-mode", "cors")
		req.Header.Set("sec-fetch-dest", "empty")
	} else {
		req.Header.Set("sec-fetch-site", "none")
		req.Header.Set("sec-fetch-mode", "navigate")
		req.Header.Set("sec-fetch-user", "?1")
		req.Header.Set("sec-fetch-dest", "document")
	}

	sessionKey := req.Header.Get("X-Claude2Api-Key")
	req.Header.Del("X-Claude2Api-Key")
	if sessionKey == "" {
		if cookie := req.Header.Get("Cookie"); cookie != "" {
			for _, part := range strings.Split(cookie, ";") {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, localTokenCookie+"=") {
					sessionKey, _ = url.QueryUnescape(strings.TrimPrefix(part, localTokenCookie+"="))
					break
				}
			}
		}
	}
	var session config.SessionInfo
	if sessionKey != "" {
		for i := range config.ConfigInstance.Sessions {
			if config.ConfigInstance.Sessions[i].SessionKey == sessionKey {
				session = config.ConfigInstance.Sessions[i]
				break
			}
		}
	}
	if session.SessionKey == "" {
		session = pickSession()
	}

	// Forward the same browser cookie state the client path uses: start from
	// the full cookie file header (cf_bm, routingHint, …) when the session has
	// one, then ensure sessionKey is present.
	cookie := session.ExtraCookie
	if !strings.Contains(cookie, "sessionKey=") {
		cookie = strings.TrimSpace(cookie)
		if cookie != "" {
			cookie += "; "
		}
		cookie += "sessionKey=" + session.SessionKey
	}
	req.Header.Set("Cookie", cookie)
	if session.DeviceID != "" {
		req.Header.Set("anthropic-device-id", session.DeviceID)
	}
	req.Header.Set("Origin", claudeBaseURL)
	req.Header.Set("Referer", claudeBaseURL+"/")

	// The browser sends Host: localhost; claude.ai expects its own host,
	// already set by Director via req.Host
}

// gatewayModifyResponse rewrites claude.ai responses so they work when served
// from localhost: strip CSP/frame guards and rewrite redirects.
func gatewayModifyResponse(resp *http.Response) error {
	// Drop security headers that break same-origin assumptions on localhost
	resp.Header.Del("Content-Security-Policy")
	resp.Header.Del("Content-Security-Policy-Report-Only")
	resp.Header.Del("X-Frame-Options")
	resp.Header.Del("Clear-Site-Data")

	// Rewrite Set-Cookie domains so claude.ai cookies apply on localhost
	if cookies := resp.Header.Values("Set-Cookie"); len(cookies) > 0 {
		resp.Header.Del("Set-Cookie")
		for _, c := range cookies {
			c = rewriteSetCookie(c)
			resp.Header.Add("Set-Cookie", c)
		}
	}

	// Rewrite redirect Location headers to stay on localhost
	if loc := resp.Header.Get("Location"); loc != "" && strings.HasPrefix(loc, claudeBaseURL) {
		resp.Header.Set("Location", strings.TrimPrefix(loc, claudeBaseURL))
	}
	return nil
}

func rewriteSetCookie(value string) string {
	parts := strings.Split(value, ";")
	kept := parts[:1]
	for _, attr := range parts[1:] {
		attr = strings.TrimSpace(attr)
		lower := strings.ToLower(attr)
		if strings.HasPrefix(lower, "domain=") || lower == "secure" || strings.HasPrefix(lower, "samesite=none") {
			continue
		}
		kept = append(kept, attr)
	}
	return strings.Join(kept, "; ")
}

// GatewayStatusHandler reports whether the gateway is enabled
func GatewayStatusHandler(gc interface{ JSON(int, interface{}) }) {
	gc.JSON(http.StatusOK, map[string]interface{}{
		"gateway": "enabled",
	})
}

var _ = io.Discard // keep io import if unused in future edits
