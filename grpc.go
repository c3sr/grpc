package grpc

import (
	"math"
	"time"

	"github.com/k0kubun/pp"

	"github.com/davecgh/go-spew/spew"
	"github.com/facebookgo/stack"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpclogrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpcRetry "github.com/grpc-ecosystem/go-grpc-middleware/retry"

	//"github.com/grpc-ecosystem/go-grpc-prometheus"
	"context"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/c3sr/tracer"
	_ "github.com/c3sr/tracer/jaeger"
	tracegrpc "github.com/c3sr/tracer/middleware/grpc"
	_ "github.com/c3sr/tracer/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

const (
	defaultWindowSize     = 65535
	initialWindowSize     = defaultWindowSize * 32 // for an RPC
	initialConnWindowSize = initialWindowSize * 16 // for a connection
)

var loggerOpts = []grpclogrus.Option{
	grpclogrus.WithDurationField(func(duration time.Duration) (key string, value interface{}) {
		return "grpc.time_ns", duration.Nanoseconds()
	}),
}

var recoveryOpts = []grpc_recovery.Option{
	grpc_recovery.WithRecoveryHandler(onPanic),
}

func onPanic(p interface{}) error {
	stack := stack.Callers(8)
	pp.Println(stack)
	log.WithField("values", spew.Sdump(p)).WithField("stack", stack).Error("paniced in grpc")
	return errors.WithStack(errors.New("recovered from grpc panic"))
}

func NewServer(service grpc.ServiceDesc) *grpc.Server {
	grpclogrus.ReplaceGrpcLogger(log)
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		grpc_recovery.UnaryServerInterceptor(recoveryOpts...),
		grpclogrus.UnaryServerInterceptor(log, loggerOpts...),
		//grpc_prometheus.UnaryServerInterceptor,
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		grpc_recovery.StreamServerInterceptor(recoveryOpts...),
		grpclogrus.StreamServerInterceptor(log, loggerOpts...),
		//grpc_prometheus.StreamServerInterceptor,
	}

	tracer := tracer.Std()
	if tracer != nil {
		unaryInterceptors = append(unaryInterceptors, tracegrpc.UnaryServerInterceptor(tracegrpc.WithTracer(tracer)))
		//unaryInterceptors = append(unaryInterceptors, otgrpc.OpenTracingServerInterceptor(tracer))
		streamInterceptors = append(streamInterceptors, tracegrpc.StreamServerInterceptor(tracegrpc.WithTracer(tracer)))
	}

	opts := []grpc.ServerOption{
		grpc_middleware.WithUnaryServerChain(unaryInterceptors...),
		grpc_middleware.WithStreamServerChain(streamInterceptors...),

		// The limiting factor for lowering the max message size is the fact
		// that a single large kv can be sent over the network in one message.
		// Our maximum kv size is unlimited, so we need this to be very large.
		grpc.MaxRecvMsgSize(math.MaxInt32),
		grpc.MaxSendMsgSize(math.MaxInt32),
		// Adjust the stream and connection window sizes. The gRPC defaults are too
		// low for high latency connections.
		grpc.InitialWindowSize(initialWindowSize),
		grpc.InitialConnWindowSize(initialConnWindowSize),
		// The default number of concurrent streams/requests on a client connection
		// is 100, while the server is unlimited. The client setting can only be
		// controlled by adjusting the server value. Set a very large value for the
		// server value so that we have no fixed limit on the number of concurrent
		// streams/requests on either the client or server.
		grpc.MaxConcurrentStreams(math.MaxInt32),
		grpc.RPCDecompressor(snappyDecompressor{}),
		// By default, gRPC disconnects clients that send "too many" pings,
		// but we don't really care about that, so configure the server to be
		// as permissive as possible.
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             time.Nanosecond,
			PermitWithoutStream: true,
		}),
		grpc.RPCCompressor(snappyCompressor{}),
		grpc.RPCDecompressor(snappyDecompressor{}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute, // https://stackoverflow.com/questions/52993259/problem-with-grpc-setup-getting-an-intermittent-rpc-unavailable-error/54703234#54703234
		}),
	}
	return grpc.NewServer(opts...)
}

func DialContext(ctx context.Context, service grpc.ServiceDesc, addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {

	retryOpts := []grpcRetry.CallOption{
		grpcRetry.WithMax(10),
		grpcRetry.WithBackoff(grpcRetry.BackoffExponential(100 * time.Millisecond)),
	}

	unaryInterceptors := []grpc.UnaryClientInterceptor{
		grpcRetry.UnaryClientInterceptor(retryOpts...),
		grpclogrus.UnaryClientInterceptor(log.WithField("dial_address", addr), loggerOpts...),
		//grpc_prometheus.UnaryClientInterceptor,
	}
	streamInterceptors := []grpc.StreamClientInterceptor{
		grpcRetry.StreamClientInterceptor(retryOpts...),
		grpclogrus.StreamClientInterceptor(log, loggerOpts...),
		//grpc_prometheus.StreamClientInterceptor,
	}

	if span := opentracing.SpanFromContext(ctx); span != nil {
		unaryInterceptors = append(unaryInterceptors, tracegrpc.UnaryClientInterceptor(tracegrpc.WithTracer(span.Tracer())))
		//unaryInterceptors = append(unaryInterceptors, otgrpc.OpenTracingClientInterceptor(span.Tracer()))
		streamInterceptors = append(streamInterceptors, tracegrpc.StreamClientInterceptor(tracegrpc.WithTracer(span.Tracer())))
	}

	dialOpts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(grpc_middleware.ChainUnaryClient(unaryInterceptors...)),
		grpc.WithStreamInterceptor(grpc_middleware.ChainStreamClient(streamInterceptors...)),

		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(math.MaxInt32), grpc.MaxCallSendMsgSize(math.MaxInt32)),

		grpc.WithInitialWindowSize(initialWindowSize),
		grpc.WithInitialConnWindowSize(initialConnWindowSize),

		grpc.WithCompressor(snappyCompressor{}),
		grpc.WithDecompressor(snappyDecompressor{}),
	}
	extra := []grpc.DialOption{}
	dialOpts = append(dialOpts, extra...)
	dialOpts = append(dialOpts, opts...)
	return grpc.DialContext(ctx, addr, dialOpts...)
}
