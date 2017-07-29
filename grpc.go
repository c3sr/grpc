package grpc

import (
	"errors"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/grpc-ecosystem/go-grpc-middleware"
	grpclogrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	"github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/rai-project/tracer"
	"google.golang.org/grpc"
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
	log.WithField("values", spew.Sdump(p)).Error("paniced in grpc")
	return errors.New("recovered from grpc panic")
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
		grpc.MaxMsgSize(500 * 1024 * 1024), // 500 MB
	}
	return grpc.NewServer(opts...)
}

func Dial(service grpc.ServiceDesc, addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {

	unaryInterceptors := []grpc.UnaryClientInterceptor{
		grpclogrus.UnaryClientInterceptor(log, loggerOpts...),
		grpc_prometheus.UnaryClientInterceptor,
	}
	streamInterceptors := []grpc.StreamClientInterceptor{
		grpclogrus.StreamClientInterceptor(log, loggerOpts...),
		grpc_prometheus.StreamClientInterceptor,
	}

	if tracer, err := tracer.New(service.ServiceName); err == nil {
		unaryInterceptors = append(unaryInterceptors, grpc_opentracing.UnaryClientInterceptor(grpc_opentracing.WithTracer(tracer)))
		streamInterceptors = append(streamInterceptors, grpc_opentracing.StreamClientInterceptor(grpc_opentracing.WithTracer(tracer)))
	}

	dialOpts := []grpc.DialOption{
		grpc.WithUnaryInterceptor(grpc_middleware.ChainUnaryClient(unaryInterceptors...)),
		grpc.WithStreamInterceptor(grpc_middleware.ChainStreamClient(streamInterceptors...)),
	}
	extra := []grpc.DialOption{}
	dialOpts = append(dialOpts, extra...)
	return grpc.Dial(addr, dialOpts...)
}
