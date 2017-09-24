package grpc

import (
	"math"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/facebookgo/stack"
	"github.com/grpc-ecosystem/go-grpc-middleware"
	grpclogrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	"github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/rai-project/tracer"
	_ "github.com/rai-project/tracer/jaeger"
	_ "github.com/rai-project/tracer/noop"
	_ "github.com/rai-project/tracer/zipkin"
	"golang.org/x/net/context"
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
	stack := stack.Callers(1)
	log.WithField("values", spew.Sdump(p)).WithField("stack", stack).Error("paniced in grpc")
	return errors.WithStack(errors.New("recovered from grpc panic"))
}

func NewServer(service grpc.ServiceDesc) *grpc.Server {
	grpclogrus.ReplaceGrpcLogger(log)
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		grpc_recovery.UnaryServerInterceptor(recoveryOpts...),
		grpclogrus.UnaryServerInterceptor(log, loggerOpts...),
		grpc_prometheus.UnaryServerInterceptor,
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		grpc_recovery.StreamServerInterceptor(recoveryOpts...),
		grpclogrus.StreamServerInterceptor(log, loggerOpts...),
		grpc_prometheus.StreamServerInterceptor,
	}

	if tracer, err := tracer.New(service.ServiceName); err == nil {
		unaryInterceptors = append(unaryInterceptors, grpc_opentracing.UnaryServerInterceptor(grpc_opentracing.WithTracer(tracer)))
		streamInterceptors = append(streamInterceptors, grpc_opentracing.StreamServerInterceptor(grpc_opentracing.WithTracer(tracer)))
	}

	opts := []grpc.ServerOption{
		grpc_middleware.WithUnaryServerChain(unaryInterceptors...),
		grpc_middleware.WithStreamServerChain(streamInterceptors...),

		// The limiting factor for lowering the max message size is the fact
		// that a single large kv can be sent over the network in one message.
		// Our maximum kv size is unlimited, so we need this to be very large.
		//
		// TODO(peter,tamird): need tests before lowering.
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
	}
	return grpc.NewServer(opts...)
}

func DialContext(ctx context.Context, service grpc.ServiceDesc, addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	unaryInterceptors := []grpc.UnaryClientInterceptor{
		grpclogrus.UnaryClientInterceptor(log.WithField("dial_address", addr), loggerOpts...),
		grpc_prometheus.UnaryClientInterceptor,
	}
	streamInterceptors := []grpc.StreamClientInterceptor{
		grpclogrus.StreamClientInterceptor(log, loggerOpts...),
		grpc_prometheus.StreamClientInterceptor,
	}

	if span := opentracing.SpanFromContext(ctx); span != nil {
		unaryInterceptors = append(unaryInterceptors, grpc_opentracing.UnaryClientInterceptor(grpc_opentracing.WithTracer(span.Tracer())))
		streamInterceptors = append(streamInterceptors, grpc_opentracing.StreamClientInterceptor(grpc_opentracing.WithTracer(span.Tracer())))
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
	return grpc.DialContext(ctx, addr, dialOpts...)
}
