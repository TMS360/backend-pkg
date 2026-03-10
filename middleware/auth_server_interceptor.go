package middleware

import (
	"context"
	"crypto/rsa"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	// Import your domain/utils where Actor and Token validation live
)

// AuthServerInterceptor extracts the JWT from incoming gRPC metadata,
// validates it, and injects the Actor into the context.
func AuthServerInterceptor(rsaPubKey *rsa.PublicKey) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Extract metadata from the incoming gRPC context
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata in request")
		}

		// Look for the 'authorization' header
		authHeaders := md.Get("authorization")
		if len(authHeaders) == 0 {
			return nil, status.Error(codes.Unauthenticated, "authorization token is not provided")
		}

		// Use your universal parser
		// authHeaders[0] will be the full "Bearer eyJhbGci..." string
		actor, err := parseAuthToken(authHeaders[0], rsaPubKey)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		// Pass the newly enriched context to the actual gRPC handler
		return handler(WithActor(ctx, actor), req)
	}
}
