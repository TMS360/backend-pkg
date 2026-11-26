package here

import "context"

type timeIndependentKey struct{}

// WithTimeIndependentRoute asks GetRoute for a route that does not depend on the
// departure time (HERE `departureTime=any`): no live traffic, so the same stops
// always give the same road and distance. Use it for miles, never for ETAs
// (DEV-4773).
func WithTimeIndependentRoute(ctx context.Context) context.Context {
	return context.WithValue(ctx, timeIndependentKey{}, true)
}

// TimeIndependentRoute reports whether ctx was marked by WithTimeIndependentRoute.
func TimeIndependentRoute(ctx context.Context) bool {
	on, _ := ctx.Value(timeIndependentKey{}).(bool)
	return on
}
