package middleware

import (
	"context"
	"crypto/rsa"

	"github.com/TMS360/backend-pkg/consts"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
		actor, err := extractActorFromIncomingMetadata(ctx, rsaPubKey)
		if err != nil {
			return nil, err
		}
		return handler(WithActor(ctx, actor), req)
	}
}

// AuthStreamServerInterceptor is the streaming counterpart of
// AuthServerInterceptor. It extracts the JWT from incoming gRPC metadata,
// validates it, and exposes an Actor-enriched context via the wrapped
// ServerStream's Context().
func AuthStreamServerInterceptor(rsaPubKey *rsa.PublicKey) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		actor, err := extractActorFromIncomingMetadata(ss.Context(), rsaPubKey)
		if err != nil {
			return err
		}
		return handler(srv, &actorServerStream{ServerStream: ss, ctx: WithActor(ss.Context(), actor)})
	}
}

type actorServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *actorServerStream) Context() context.Context { return s.ctx }

func extractActorFromIncomingMetadata(ctx context.Context, rsaPubKey *rsa.PublicKey) (*consts.Actor, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata in request")
	}
	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return nil, status.Error(codes.Unauthenticated, "authorization token is not provided")
	}
	actor, err := parseAuthToken(authHeaders[0], rsaPubKey)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
	}
	return actor, nil
}
