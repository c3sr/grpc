package web

import (
	"fmt"

	"google.golang.org/grpc"
)

func ListEndpoints(service grpc.ServiceDesc) []string {
	ret := []string{}
	for _, methodInfo := range service.Methods {
		fullResource := fmt.Sprintf("/%s/%s", service.ServiceName, methodInfo.MethodName)
		ret = append(ret, fullResource)
	}
	return ret
}
