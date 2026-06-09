package middleware

import (
	"context"
	"crypto/rsa"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthServerInterceptor extracts the JWT from incoming gRPC metadata,
// validates it, and injects the Actor into the context. If the call carries
// an x-internal-token matching internalToken (GRPC_INTERNAL_TOKEN, shared
// across services), a system Actor is built from x-actor-id and x-company-id
// instead of parsing a JWT.
func AuthServerInterceptor(rsaPubKey *rsa.PublicKey, internalToken string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		actor, err := extractActorFromIncomingMetadata(ctx, rsaPubKey, internalToken)
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
func AuthStreamServerInterceptor(rsaPubKey *rsa.PublicKey, internalToken string) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		actor, err := extractActorFromIncomingMetadata(ss.Context(), rsaPubKey, internalToken)
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

func extractActorFromIncomingMetadata(ctx context.Context, rsaPubKey *rsa.PublicKey, internalToken string) (*consts.Actor, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata in request")
	}
	if svc := md.Get("x-internal-token"); len(svc) > 0 {
		if internalToken == "" || svc[0] != internalToken {
			return nil, status.Error(codes.Unauthenticated, "invalid internal token")
		}
		return buildSystemActorFromMetadata(md), nil
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

// buildSystemActorFromMetadata constructs a system Actor from x-actor-id and
// x-company-id headers carried alongside x-internal-token. Used for
// service-to-service calls where the originating actor has no JWT (background
// jobs, Kafka handlers) but still needs identity for tenant scoping and audit.
func buildSystemActorFromMetadata(md metadata.MD) *consts.Actor {
	actor := &consts.Actor{IsSystem: true, Claims: &consts.UserClaims{}}
	if v := md.Get("x-actor-id"); len(v) > 0 {
		if id, err := uuid.Parse(v[0]); err == nil {
			actor.ID = id
			actor.Claims.UserID = id
		}
	}
	if v := md.Get("x-company-id"); len(v) > 0 {
		if cid, err := uuid.Parse(v[0]); err == nil {
			actor.Claims.CompanyID = &cid
		}
	}
	actor.Claims.PopulateMaps()
	return actor
}
