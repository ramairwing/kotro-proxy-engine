// Package middleware provides composable HTTP middleware for the proxy pipeline.
package middleware

import "net/http"

// Middleware wraps an http.Handler with additional request/response processing.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware in declaration order: first middleware is outermost.
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
