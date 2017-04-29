package middlware

import (
	"github.com/Sirupsen/logrus"
	grpclogrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	"google.golang.org/grpc"
)

func Logrus(logrusEntry *logrus.Entry) grpc.UnaryServerInterceptor {
	grpclogrus.ReplaceGrpcLogger(logrusEntry)
	return grpclogrus.UnaryServerInterceptor(logrusEntry)
}
