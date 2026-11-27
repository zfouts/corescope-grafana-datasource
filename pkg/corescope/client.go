package corescope

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"golang.org/x/sync/singleflight"

	"github.com/zfouts/corescope-grafana-datasource/pkg/models"
)

var _ API = (*Client)(nil)

const (
	userAgent = "corescope-grafana-datasource"

	maxBodyBytes      = 8 << 20  // every endpoint but /api/nodes
	maxNodesBodyBytes = 32 << 20 // /api/nodes
	maxErrorBodyBytes = 64 << 10 // a non-2xx body is only read for {"error": ...}
	maxDrainBytes     = 64 << 10 // bounded drain so the connection can be reused

	maxAttempts    = 3
	backoffInitial = 250 * time.Millisecond
	backoffMax     = 2 * time.Second

	cacheTTL           = 5 * time.Second
	cacheMaxEntries    = 256
	cacheMaxBytes      = 64 << 20 // total body bytes held by one Client's cache
	cacheMaxEntryBytes = 4 << 20  // larger bodies are never cached

	maxMessageRunes = 200
)

var (
	observerIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	pubkeyRe     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Client is the read-only CoreScope HTTP client. It is safe for concurrent
// use. One Client per datasource instance; Close it when the instance goes.
type Client struct {
	base    string // validated, no trailing "/"
	timeout time.Duration
	http    *http.Client
	// transport is the *http.Transport under the SDK middleware chain. The
	// chain's outer RoundTripper has no CloseIdleConnections, so
	// http.Client.CloseIdleConnections would be a no-op; Close uses this.
	transport *http.Transport

	basicUser, basicPass string
	bearer               string

	sf    singleflight.Group
	cache *respCache

	// sharedRetryHook, when set (tests only), runs each time a follower
	// re-fetches after its leader's flight was cancelled.
	sharedRetryHook func()
}

// NewClient validates s (normalizing it in place) and builds a Client.
func NewClient(s *models.PluginSettings) (*Client, error) {
	if err := models.ValidateSettings(s); err != nil {
		return nil, err
	}
	timeout := time.Duration(s.TimeoutSeconds) * time.Second

	to := httpclient.DefaultTimeoutOptions
	to.Timeout = timeout // http.Client.Timeout and Transport.ResponseHeaderTimeout
	to.TLSHandshakeTimeout = 10 * time.Second

	var transport *http.Transport
	hc, err := httpclient.New(httpclient.Options{
		Timeouts: &to,
		ConfigureTransport: func(_ httpclient.Options, t *http.Transport) {
			transport = t
		},
		TLS: &httpclient.TLSOptions{InsecureSkipVerify: s.TLSSkipVerify, MinVersion: tls.VersionTLS12},
		ConfigureClient: func(_ httpclient.Options, c *http.Client) {
			c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		},
	})
	if err != nil || transport == nil {
		return nil, errors.New("corescope: could not build HTTP client")
	}

	c := &Client{
		base:      s.URL,
		timeout:   timeout,
		http:      hc,
		transport: transport,
		cache:     newRespCache(cacheTTL, cacheMaxEntries, time.Now),
	}
	if s.BasicAuthUser != "" {
		c.basicUser = s.BasicAuthUser
		if s.Secrets != nil {
			c.basicPass = s.Secrets.BasicAuthPassword
		}
	} else if s.Secrets != nil {
		c.bearer = s.Secrets.BearerToken
	}
	return c, nil
}

// Close releases idle connections. In-flight requests are not interrupted.
func (c *Client) Close() { c.transport.CloseIdleConnections() }

// ─── API methods ──────────────────────────────────────────────────────────────

func (c *Client) Healthz(ctx context.Context) (*Healthz, error) {
	return getJSON[Healthz](ctx, c, "/api/healthz", nil, maxBodyBytes)
}

func (c *Client) Stats(ctx context.Context) (*Stats, error) {
	return getJSON[Stats](ctx, c, "/api/stats", nil, maxBodyBytes)
}

func (c *Client) Health(ctx context.Context) (*Health, error) {
	return getJSON[Health](ctx, c, "/api/health", nil, maxBodyBytes)
}

func (c *Client) Perf(ctx context.Context) (*Perf, error) {
	return getJSON[Perf](ctx, c, "/api/perf", nil, maxBodyBytes)
}

func (c *Client) MqttStatus(ctx context.Context) (*MqttStatus, error) {
	return getJSON[MqttStatus](ctx, c, "/api/mqtt/status", nil, maxBodyBytes)
}

func (c *Client) RF(ctx context.Context, w Window) (*RFAnalytics, error) {
	return getJSON[RFAnalytics](ctx, c, "/api/analytics/rf", windowQuery(w), maxBodyBytes)
}

func (c *Client) Retransmissions(ctx context.Context, w Window) (*Retransmissions, error) {
	return getJSON[Retransmissions](ctx, c, "/api/analytics/retransmissions", windowQuery(w), maxBodyBytes)
}

func (c *Client) ObserverMetrics(ctx context.Context, observerID string, from, to time.Time, resolution string) (*ObserverMetrics, error) {
	if !observerIDRe.MatchString(observerID) {
		return nil, &Error{Status: http.StatusBadRequest, Path: "/api/observers/{id}/metrics", Message: "invalid observer id"}
	}
	q := url.Values{}
	setTime(q, "since", from)
	setTime(q, "until", to)
	setString(q, "resolution", resolution)
	return getJSON[ObserverMetrics](ctx, c, "/api/observers/"+url.PathEscape(observerID)+"/metrics", q, maxBodyBytes)
}

func (c *Client) NodeAnalytics(ctx context.Context, pubkey string, days int) (*NodeAnalytics, error) {
	pk, ok := normPubkey(pubkey)
	if !ok {
		return nil, &Error{Status: http.StatusBadRequest, Path: "/api/nodes/{pubkey}/analytics", Message: "invalid pubkey"}
	}
	return getJSON[NodeAnalytics](ctx, c, "/api/nodes/"+url.PathEscape(pk)+"/analytics", daysQuery(days), maxBodyBytes)
}

func (c *Client) NodeBattery(ctx context.Context, pubkey string, days int) (*NodeBattery, error) {
	pk, ok := normPubkey(pubkey)
	if !ok {
		return nil, &Error{Status: http.StatusBadRequest, Path: "/api/nodes/{pubkey}/battery", Message: "invalid pubkey"}
	}
	return getJSON[NodeBattery](ctx, c, "/api/nodes/"+url.PathEscape(pk)+"/battery", daysQuery(days), maxBodyBytes)
}

func (c *Client) Nodes(ctx context.Context, nq NodeQuery) (*NodeList, error) {
	q := url.Values{}
	if nq.Limit != 0 {
		q.Set("limit", strconv.Itoa(nq.Limit))
	}
	setString(q, "role", nq.Role)
	setString(q, "region", nq.Region)
	setString(q, "search", nq.Search)
	return getJSON[NodeList](ctx, c, "/api/nodes", q, maxNodesBodyBytes)
}

func (c *Client) Observers(ctx context.Context) (*ObserverList, error) {
	return getJSON[ObserverList](ctx, c, "/api/observers", nil, maxBodyBytes)
}

func (c *Client) Regions(ctx context.Context) (Regions, error) {
	r, err := getJSON[Regions](ctx, c, "/api/config/regions", nil, maxBodyBytes)
	if err != nil {
		return nil, err
	}
	return *r, nil
}

// ─── Query helpers ────────────────────────────────────────────────────────────

func windowQuery(w Window) url.Values {
	q := url.Values{}
	setTime(q, "from", w.From)
	setTime(q, "to", w.To)
	setString(q, "region", w.Region)
	return q
}

func daysQuery(days int) url.Values {
	q := url.Values{}
	if days != 0 {
		q.Set("days", strconv.Itoa(days))
	}
	return q
}

func setTime(q url.Values, k string, t time.Time) {
	if !t.IsZero() {
		q.Set(k, t.UTC().Format(time.RFC3339))
	}
}

func setString(q url.Values, k, v string) {
	if v != "" {
		q.Set(k, v)
	}
}

func normPubkey(s string) (string, bool) {
	pk := strings.ToLower(s)
	return pk, pubkeyRe.MatchString(pk)
}

// ─── Request pipeline ─────────────────────────────────────────────────────────

// getJSON fetches path (already escaped) with q and decodes into a fresh T.
func getJSON[T any](ctx context.Context, c *Client, path string, q url.Values, limit int64) (*T, error) {
	body, err := c.get(ctx, path, q, limit)
	if err != nil {
		return nil, err
	}
	v := new(T)
	if err := json.Unmarshal(body, v); err != nil {
		backend.Logger.Debug("corescope: JSON decode failed", "path", path)
		return nil, &Error{Status: http.StatusBadGateway, Path: path, Message: "invalid JSON from upstream"}
	}
	return v, nil
}

// get returns the raw 2xx JSON body for path, from the cache when fresh,
// otherwise through one shared upstream fetch per URL. The returned slice is
// shared with the cache and other callers: read it, never write it.
func (c *Client) get(ctx context.Context, path string, q url.Values, limit int64) ([]byte, error) {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	if b, ok := c.cache.get(u); ok {
		return b, nil
	}

	// A follower that joined a fetch whose leader was cancelled gets the
	// leader's context error; it then fetches again on its own context.
	for i := 0; i < 2; i++ {
		ch := c.sf.DoChan(u, func() (interface{}, error) {
			if b, ok := c.cache.get(u); ok {
				return b, nil
			}
			b, err := c.fetch(ctx, u, path, limit)
			if err == nil {
				c.cache.put(u, b)
			}
			return b, err
		})
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case r := <-ch:
			if r.Err == nil {
				return r.Val.([]byte), nil
			}
			if isCtxErr(r.Err) && ctx.Err() == nil && r.Shared {
				if c.sharedRetryHook != nil {
					c.sharedRetryHook()
				}
				continue
			}
			return nil, copyErr(r.Err)
		}
	}
	return nil, &Error{Status: http.StatusGatewayTimeout, Path: path, Message: "upstream request was cancelled"}
}

// fetch performs up to maxAttempts GETs of u within the per-call budget.
func (c *Client) fetch(parent context.Context, u, path string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, c.timeout)
	defer cancel()

	var lastErr error
	for attempt := 1; ; attempt++ {
		body, retryAfter, retry, err := c.attempt(ctx, u, path, limit)
		if err == nil {
			return body, nil
		}
		if parent.Err() != nil || isCtxErr(err) {
			return nil, c.ctxFailure(parent, path)
		}
		lastErr = err
		if !retry || attempt >= maxAttempts {
			return nil, lastErr
		}

		wait := backoff(attempt)
		if retryAfter > 0 {
			wait = retryAfter
		}
		if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= wait {
			// Waiting would exhaust the budget; fail now with the real cause.
			return nil, lastErr
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, c.ctxFailure(parent, path)
		case <-t.C:
		}
	}
}

// ctxFailure maps a done context to the caller's own error when the caller
// cancelled, or to a timeout *Error when only the per-call budget ran out.
func (c *Client) ctxFailure(parent context.Context, path string) error {
	if err := parent.Err(); err != nil {
		return err
	}
	return &Error{Status: http.StatusGatewayTimeout, Path: path, Message: "upstream timed out"}
}

// attempt performs one GET. retry reports whether the failure is retryable
// (503 or connection refused); retryAfter is a 503's Retry-After, if any.
func (c *Client) attempt(ctx context.Context, u, path string, limit int64) (body []byte, retryAfter time.Duration, retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, false, &Error{Status: http.StatusBadRequest, Path: path, Message: "invalid request"}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if c.basicUser != "" {
		req.SetBasicAuth(c.basicUser, c.basicPass)
	} else if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		e, refused := transportError(path, err)
		return nil, 0, refused, e
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))
		_ = resp.Body.Close()
	}()

	mediaType := ""
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		if mt, _, perr := mime.ParseMediaType(ct); perr == nil {
			mediaType = mt
		}
	}

	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		backend.Logger.Debug("corescope: upstream redirected", "path", path, "status", resp.StatusCode)
		return nil, 0, false, &Error{Status: resp.StatusCode, Path: path, Message: "upstream redirected; check the URL"}

	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		msg := ""
		if mediaType == "application/json" {
			msg = errorMessage(resp.Body)
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		if msg == "" {
			msg = "upstream error"
		}
		backend.Logger.Debug("corescope: upstream error", "path", path, "status", resp.StatusCode, "mediaType", mediaType)
		if resp.StatusCode == http.StatusServiceUnavailable {
			retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
			retry = true
		}
		return nil, retryAfter, retry, &Error{Status: resp.StatusCode, Path: path, Message: msg}

	case mediaType != "application/json":
		shown := mediaType
		if shown == "" {
			shown = "without a JSON content type"
		}
		backend.Logger.Debug("corescope: upstream returned non-JSON", "path", path, "status", resp.StatusCode, "mediaType", mediaType)
		return nil, 0, false, &Error{
			Status:  http.StatusBadGateway,
			Path:    path,
			Message: "upstream returned " + strconv.Itoa(resp.StatusCode) + " " + shown + "; the URL may not point at a CoreScope API",
		}
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, false, ctx.Err()
		}
		backend.Logger.Debug("corescope: reading upstream body failed", "path", path, "error", err.Error())
		return nil, 0, false, &Error{Status: http.StatusBadGateway, Path: path, Message: "error reading upstream response"}
	}
	if int64(len(b)) > limit {
		backend.Logger.Debug("corescope: upstream body over limit", "path", path, "limit", limit)
		return nil, 0, false, &Error{Status: http.StatusBadGateway, Path: path, Message: "upstream response too large"}
	}
	return b, 0, false, nil
}

// transportError maps a failed Do to a fixed-text *Error. refused reports a
// connection refused, the only transport failure worth retrying.
func transportError(path string, err error) (e error, refused bool) {
	inner := err
	var ue *url.Error
	if errors.As(err, &ue) {
		inner = ue.Err // drop the URL, which carries query values
	}
	if isCtxErr(err) {
		return inner, false
	}
	backend.Logger.Debug("corescope: upstream request failed", "path", path, "error", inner.Error())

	msg := "upstream request failed"
	status := http.StatusBadGateway
	var (
		dnsErr   *net.DNSError
		unkAuth  x509.UnknownAuthorityError
		hostErr  x509.HostnameError
		certErr  x509.CertificateInvalidError
		recErr   tls.RecordHeaderError
		verifErr *tls.CertificateVerificationError
		netErr   net.Error
	)
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		msg, refused = "connection refused", true
	case errors.As(err, &dnsErr):
		msg = "host not found"
	case errors.As(err, &verifErr), errors.As(err, &unkAuth), errors.As(err, &hostErr), errors.As(err, &certErr):
		msg = "TLS certificate verification failed"
	case errors.As(err, &recErr):
		msg = "TLS handshake failed; the URL may use the wrong scheme"
	case errors.As(err, &netErr) && netErr.Timeout():
		msg, status = "upstream timed out", http.StatusGatewayTimeout
	}
	return &Error{Status: status, Path: path, Message: msg}, refused
}

func isCtxErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// copyErr gives each singleflight caller its own *Error.
func copyErr(err error) error {
	var e *Error
	if errors.As(err, &e) {
		cp := *e
		return &cp
	}
	return err
}

// errorMessage extracts CoreScope's {"error": "..."} text, sanitized.
func errorMessage(r io.Reader) string {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(r, maxErrorBodyBytes)).Decode(&body); err != nil {
		return ""
	}
	return sanitizeMessage(body.Error)
}

// sanitizeMessage strips control and format characters (Cc, Cf: terminal
// escapes, bidi overrides) and invalid UTF-8, and truncates to
// maxMessageRunes.
func sanitizeMessage(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == unicode.ReplacementChar {
			continue
		}
		if n == maxMessageRunes {
			break
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

// parseRetryAfter reads a delay-seconds Retry-After; anything else is 0.
func parseRetryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs <= 0 {
		return 0
	}
	if secs > 3600 {
		secs = 3600
	}
	return time.Duration(secs) * time.Second
}

// backoff is the wait after the given failed attempt (1-based): exponential
// from backoffInitial, capped at backoffMax, with jitter in [d/2, d].
func backoff(attempt int) time.Duration {
	d := backoffInitial << (attempt - 1)
	if d > backoffMax || d <= 0 {
		d = backoffMax
	}
	half := d / 2
	return half + rand.N(half+1)
}

// ─── Response cache ───────────────────────────────────────────────────────────

type cacheEntry struct {
	body    []byte
	expires time.Time
}

// respCache is a small TTL cache of raw 2xx bodies keyed by full URL, bounded
// by entry count (max) and by total body bytes (maxBytes). Bodies over
// maxEntryBytes are not cached. Invariant: bytes == sum of len(e.body).
type respCache struct {
	mu            sync.Mutex
	ttl           time.Duration
	max           int
	maxBytes      int
	maxEntryBytes int
	now           func() time.Time
	entries       map[string]cacheEntry
	bytes         int
}

func newRespCache(ttl time.Duration, max int, now func() time.Time) *respCache {
	return &respCache{
		ttl:           ttl,
		max:           max,
		maxBytes:      cacheMaxBytes,
		maxEntryBytes: cacheMaxEntryBytes,
		now:           now,
		entries:       make(map[string]cacheEntry),
	}
}

func (rc *respCache) get(k string) ([]byte, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	e, ok := rc.entries[k]
	if !ok {
		return nil, false
	}
	if !rc.now().Before(e.expires) {
		rc.remove(k)
		return nil, false
	}
	return e.body, true
}

func (rc *respCache) put(k string, b []byte) {
	if rc.ttl <= 0 || rc.max <= 0 {
		return
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	// Any older body for k is superseded, cached or not.
	rc.remove(k)
	if len(b) > rc.maxEntryBytes || len(b) > rc.maxBytes {
		return
	}

	now := rc.now()
	if len(rc.entries)+1 > rc.max || rc.bytes+len(b) > rc.maxBytes {
		for key, e := range rc.entries {
			if !now.Before(e.expires) {
				rc.remove(key)
			}
		}
	}
	// Evict soonest-expiring until both bounds hold. Each pass is O(max) with
	// max = 256, and only runs on insert into a full cache.
	for len(rc.entries) > 0 && (len(rc.entries)+1 > rc.max || rc.bytes+len(b) > rc.maxBytes) {
		var oldestKey string
		var oldest time.Time
		for key, e := range rc.entries {
			if oldestKey == "" || e.expires.Before(oldest) {
				oldestKey, oldest = key, e.expires
			}
		}
		rc.remove(oldestKey)
	}
	rc.entries[k] = cacheEntry{body: b, expires: now.Add(rc.ttl)}
	rc.bytes += len(b)
}

// remove deletes k and keeps bytes in step. Caller holds mu.
func (rc *respCache) remove(k string) {
	if e, ok := rc.entries[k]; ok {
		rc.bytes -= len(e.body)
		delete(rc.entries, k)
	}
}

func (rc *respCache) len() int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return len(rc.entries)
}

func (rc *respCache) size() int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.bytes
}
