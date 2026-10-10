package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config holds connection settings for an S3-compatible object store.
type S3Config struct {
	Endpoint  string
	Bucket    string
	Prefix    string
	AccessKey string
	SecretKey string
	Region    string
}

// S3Target stores backups in an S3-compatible object store.
type S3Target struct {
	cfg    S3Config
	client *s3.Client
}

func NewS3Target(cfg S3Config) (*S3Target, error) {
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	awsCfg := aws.Config{
		Region: cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		),
	}

	opts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		restrict := !allowPrivateS3()
		if restrict && s3EndpointProxied(cfg.Endpoint) {
			// Through a proxy the dialer only sees the proxy address, so
			// check the endpoint host here. This also covers imported
			// config that never passed ValidateS3Endpoint.
			if err := ValidateS3Endpoint(context.Background(), cfg.Endpoint); err != nil {
				return nil, err
			}
		}
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
			if restrict {
				// Custom endpoints come from user config (or imported
				// config that never passed ValidateS3Endpoint), so the
				// dialer checks every IP it connects to directly.
				// Connections to a configured proxy skip that check; the
				// endpoint was validated above instead. A blocked dial is
				// not worth retrying.
				o.HTTPClient = restrictedS3HTTPClient()
				o.Retryer = retry.NewStandard(func(so *retry.StandardOptions) {
					so.Retryables = append([]retry.IsErrorRetryable{
						retry.IsErrorRetryableFunc(func(err error) aws.Ternary {
							if errors.Is(err, ErrS3EndpointBlocked) {
								return aws.FalseTernary
							}
							return aws.UnknownTernary
						}),
					}, so.Retryables...)
				})
			}
		})
	}

	client := s3.NewFromConfig(awsCfg, opts...)
	return &S3Target{cfg: cfg, client: client}, nil
}

func (t *S3Target) Type() string { return "s3" }

func (t *S3Target) Test(ctx context.Context) error {
	_, err := t.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(t.cfg.Bucket),
	})
	if err != nil {
		return fmt.Errorf("s3 head bucket %q: %w", t.cfg.Bucket, err)
	}
	return nil
}

func (t *S3Target) key(filename string) string {
	if t.cfg.Prefix != "" {
		return t.cfg.Prefix + "/" + filename
	}
	return filename
}

func (t *S3Target) Upload(ctx context.Context, filename string, data io.Reader) (string, int64, error) {
	// Use the manager Uploader so we can stream a non-seekable reader
	// (pg_dump/tar stdout piped through a gzip writer is not seekable,
	// which PutObject's SHA-256 computation requires).
	cr := &countReader{r: data}
	key := t.key(filename)
	uploader := manager.NewUploader(t.client)
	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(key),
		Body:   cr,
	})
	if err != nil {
		return "", 0, fmt.Errorf("s3 put: %w", err)
	}
	return key, cr.n, nil
}

func (t *S3Target) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	out, err := t.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get: %w", err)
	}
	return out.Body, nil
}

func (t *S3Target) Delete(ctx context.Context, path string) error {
	_, err := t.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return fmt.Errorf("s3 delete: %w", err)
	}
	return nil
}

// PresignedURL returns a pre-signed GET URL valid for the given duration.
func (t *S3Target) PresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	presigner := s3.NewPresignClient(t.client)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("s3 presign: %w", err)
	}
	return req.URL, nil
}

// countReader wraps an io.Reader and counts bytes read.
type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// AllowPrivateS3Env, when set to "1", lets custom S3 endpoints resolve to
// private/loopback/reserved addresses (e.g. a MinIO on the same host).
const AllowPrivateS3Env = "SIMPLEDEPLOY_ALLOW_PRIVATE_S3"

// ErrS3EndpointBlocked is returned (wrapped) when a custom S3 endpoint
// resolves to a private or reserved address.
var ErrS3EndpointBlocked = errors.New("S3 endpoint points to a private or reserved network address")

func allowPrivateS3() bool { return os.Getenv(AllowPrivateS3Env) == "1" }

// s3LookupIPAddr resolves endpoint hosts. Tests swap it to fake DNS.
var s3LookupIPAddr = net.DefaultResolver.LookupIPAddr

// s3ReservedRanges captures CIDRs not covered by net.IP.IsPrivate /
// IsLoopback / IsLinkLocal* / IsMulticast / IsUnspecified. Mirrors the
// webhook dispatcher's list in internal/alerts.
var s3ReservedRanges = func() []*net.IPNet {
	cidrs := []string{
		"0.0.0.0/8",
		"100.64.0.0/10",   // CGNAT
		"192.0.0.0/24",    // IETF
		"192.0.2.0/24",    // TEST-NET-1
		"198.18.0.0/15",   // benchmarking
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"240.0.0.0/4",     // class E + broadcast
		"::/96",           // IPv4-compatible (deprecated), incl. :: and ::1
		"64:ff9b:1::/48",  // NAT64 local-use prefix (RFC 8215)
		"100::/64",        // IPv6 discard
		"2001:db8::/32",   // IPv6 documentation
		"fc00::/7",        // IPv6 unique-local
		"fec0::/10",       // IPv6 site-local (deprecated)
	}
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}()

// nat64WellKnown is the RFC 6052 NAT64 prefix. DNS64 resolvers on
// IPv6-only hosts map every IPv4 target into it.
var nat64WellKnown = &net.IPNet{IP: net.ParseIP("64:ff9b::"), Mask: net.CIDRMask(96, 128)}

// isReservedIP reports whether ip is loopback, private, link-local,
// multicast, unspecified, or in another reserved range. Addresses in the
// NAT64 well-known prefix are judged by the IPv4 address they embed.
func isReservedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if len(ip) == net.IPv6len && ip.To4() == nil && nat64WellKnown.Contains(ip) {
		return isReservedIP(net.IP(ip[12:16]))
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, n := range s3ReservedRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func blockedS3Error(host string, ip net.IP) error {
	return fmt.Errorf("%w: %q resolves to %s. Set %s=1 on the SimpleDeploy server to allow private endpoints such as a local MinIO",
		ErrS3EndpointBlocked, host, ip, AllowPrivateS3Env)
}

func resolveS3Host(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	addrs, err := s3LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no addresses found for %q", host)
	}
	return ips, nil
}

// ValidateS3Endpoint checks a custom S3 endpoint before it is saved. Empty
// (AWS default) is always allowed. Otherwise it must be an http(s) URL and,
// unless SIMPLEDEPLOY_ALLOW_PRIVATE_S3=1, every address the host resolves
// to must be public. The connection-time dialer re-checks this.
func ValidateS3Endpoint(ctx context.Context, endpoint string) error {
	if endpoint == "" {
		return nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("S3 endpoint %q must be a full http:// or https:// address, e.g. https://s3.example.com", endpoint)
	}
	if allowPrivateS3() {
		return nil
	}
	host := u.Hostname()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := resolveS3Host(ctx, host)
	if err != nil {
		return fmt.Errorf("could not resolve S3 endpoint host %q: %w", host, err)
	}
	for _, ip := range ips {
		if isReservedIP(ip) {
			return blockedS3Error(host, ip)
		}
	}
	return nil
}

// s3DialBlocked decides whether the S3 dialer may connect to ip. Tests
// swap it to exercise the allowed path against a loopback server.
var s3DialBlocked = isReservedIP

// s3DialControl runs after DNS resolution and before connect(), once per
// connection attempt, so it judges the exact IP being dialed. That keeps
// DNS rebinding covered while the stock dialer handles Happy Eyeballs,
// dual-stack fallback and timeouts.
func s3DialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if s3DialBlocked(ip) {
		if ip == nil {
			return fmt.Errorf("%w: cannot check address %q. Set %s=1 on the SimpleDeploy server to allow private endpoints such as a local MinIO",
				ErrS3EndpointBlocked, address, AllowPrivateS3Env)
		}
		return fmt.Errorf("%w (%s). Set %s=1 on the SimpleDeploy server to allow private endpoints such as a local MinIO",
			ErrS3EndpointBlocked, ip, AllowPrivateS3Env)
	}
	return nil
}

// s3SafeDialContext dials with the SDK's default timeouts. Every connection
// goes through s3DialControl except connections to an address isProxy
// reports as a configured proxy: the proxy resolves the endpoint itself,
// and NewS3Target has already run ValidateS3Endpoint on the endpoint host.
func s3SafeDialContext(isProxy func(addr string) bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	direct := &net.Dialer{
		Timeout:   awshttp.DefaultDialConnectTimeout,
		KeepAlive: awshttp.DefaultDialKeepAliveTimeout,
	}
	guarded := &net.Dialer{
		Timeout:   awshttp.DefaultDialConnectTimeout,
		KeepAlive: awshttp.DefaultDialKeepAliveTimeout,
		Control:   s3DialControl,
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if isProxy != nil && isProxy(addr) {
			return direct.DialContext(ctx, network, addr)
		}
		return guarded.DialContext(ctx, network, addr)
	}
}

// restrictS3Transport installs the IP-checking dialer on tr while still
// honouring the proxy func (normally the proxy environment variables).
// The host:port of every proxy it returns is remembered so only the
// connection to that proxy skips the IP check; direct connections (hosts
// matched by NO_PROXY, loopback names, redirects) are always checked.
func restrictS3Transport(tr *http.Transport, proxy func(*http.Request) (*url.URL, error)) {
	var proxies sync.Map
	tr.Proxy = func(req *http.Request) (*url.URL, error) {
		u, err := proxy(req)
		if err == nil && u != nil {
			proxies.Store(proxyDialAddr(u), struct{}{})
		}
		return u, err
	}
	tr.DialContext = s3SafeDialContext(func(addr string) bool {
		_, ok := proxies.Load(addr)
		return ok
	})
}

// proxyDialAddr returns the address http.Transport dials for proxy u
// (host:port, with the scheme's default port).
func proxyDialAddr(u *url.URL) string {
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "socks5", "socks5h":
			port = "1080"
		default:
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}

// s3EnvProxy picks the proxy for S3 requests from the environment
// (HTTP_PROXY, HTTPS_PROXY, NO_PROXY). Tests swap it.
var s3EnvProxy = http.ProxyFromEnvironment

// s3EndpointProxied reports whether requests to endpoint would go through
// a proxy from the environment. A proxy setting that cannot be parsed
// counts as proxied.
func s3EndpointProxied(endpoint string) bool {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	u, err := s3EnvProxy(req)
	return err != nil || u != nil
}

// restrictedS3HTTPClient is the SDK's default client with the IP-checking
// dialer. Proxy environment variables (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)
// keep working as with the stock client.
func restrictedS3HTTPClient() *awshttp.BuildableClient {
	return awshttp.NewBuildableClient().WithTransportOptions(func(tr *http.Transport) {
		restrictS3Transport(tr, s3EnvProxy)
	})
}
