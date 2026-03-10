package middleware

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// AuthClientInterceptor intercepts outgoing gRPC calls and automatically
// attaches the user's JWT to the outbound metadata if an Actor is present in the context.
func AuthClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		actor, err := GetActor(ctx)
		if err == nil && actor != nil && actor.Token != nil && *actor.Token != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+*actor.Token)
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
