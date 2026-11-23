package middleware

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// AuthClientInterceptor intercepts outgoing gRPC calls and automatically
// attaches the user's JWT to the outbound metadata if an Actor is present in the context.
//
// When the actor is a system actor (Kafka handlers, cron jobs, service-to-service
// calls without an end-user), it falls back to the shared internalToken and
// propagates the actor's identity (ID and CompanyID) so the receiving side can
// reconstruct the actor. internalToken is the value of GRPC_INTERNAL_TOKEN,
// passed in by the caller (typically from config).
func AuthClientInterceptor(internalToken string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		actor, _ := GetActor(ctx)
		switch {
		case actor != nil && actor.Token != nil && *actor.Token != "":
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+*actor.Token)
		case actor != nil && actor.IsSystem && internalToken != "":
			ctx = metadata.AppendToOutgoingContext(ctx,
				"x-internal-token", internalToken,
				"x-actor-id", actor.ID.String(),
			)
			if cid := actor.GetCompanyID(); cid != nil {
				ctx = metadata.AppendToOutgoingContext(ctx, "x-company-id", cid.String())
			}
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func AuthStreamClientInterceptor(internalToken string) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		actor, _ := GetActor(ctx)
		switch {
		case actor != nil && actor.Token != nil && *actor.Token != "":
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+*actor.Token)
		case actor != nil && actor.IsSystem && internalToken != "":
			ctx = metadata.AppendToOutgoingContext(ctx,
				"x-internal-token", internalToken,
				"x-actor-id", actor.ID.String(),
			)
			if cid := actor.GetCompanyID(); cid != nil {
				ctx = metadata.AppendToOutgoingContext(ctx, "x-company-id", cid.String())
			}
		}
		return streamer(ctx, desc, cc, method, opts...)
	}
}
