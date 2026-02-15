package entrypoint

import (
	"github.com/yusing/godoxy/internal/types"
)

// Entrypoint is the main HTTP entry point for the proxy: it performs domain-based
// route lookup, applies middleware, manages HTTP/HTTPS server lifecycle, and
// exposes route pools and health info. Route providers register routes via
// StartAddRoute; request handling uses the route pools to resolve targets.
type Entrypoint interface {
	// SupportProxyProtocol reports whether the entrypoint is configured to accept
	// PROXY protocol (v1/v2) on incoming connections. When true, servers expect
	// the PROXY header before reading HTTP.
	SupportProxyProtocol() bool

	// DisablePoolsLog sets whether add/del logging for route pools is disabled.
	// When v is true, logging for HTTP, stream, and excluded route pools is
	// turned off; when false, it is turned on. Affects all existing and future
	// pool operations until called again.
	DisablePoolsLog(v bool)

	GetRoute(alias string) (types.Route, bool)
	// StartAddRoute registers the route with the entrypoint. It is synchronous:
	// it does not return until the route is registered or an error occurs. For
	// HTTP routes, a server for the route's listen address is created and
	// started if needed. For stream routes, ListenAndServe is invoked and the
	// route is added to the pool only on success. Excluded routes are added to
	// the excluded pool only. Returns an error on listen/bind failure, stream
	// listen failure, or unsupported route type.
	StartAddRoute(r types.Route) error
	IterRoutes(yield func(r types.Route) bool)
	NumRoutes() int
	RoutesByProvider() map[string][]types.Route

	// HTTPRoutes returns a read-only view of all HTTP routes (across listen addrs).
	HTTPRoutes() PoolLike[types.HTTPRoute]
	// StreamRoutes returns a read-only view of all stream (e.g. TCP/UDP) routes.
	StreamRoutes() PoolLike[types.StreamRoute]
	// ExcludedRoutes returns the read-write pool of excluded routes (e.g. disabled).
	ExcludedRoutes() RWPoolLike[types.Route]

	GetHealthInfo() map[string]types.HealthInfo
	GetHealthInfoWithoutDetail() map[string]types.HealthInfoWithoutDetail
	GetHealthInfoSimple() map[string]types.HealthStatus
}

type PoolLike[Route types.Route] interface {
	Get(alias string) (Route, bool)
	Iter(yield func(alias string, r Route) bool)
	Size() int
}

type RWPoolLike[Route types.Route] interface {
	PoolLike[Route]
	Add(r Route)
	Del(r Route)
}
