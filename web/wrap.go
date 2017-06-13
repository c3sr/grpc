package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/rs/cors"
	"google.golang.org/grpc"
)

type wrapper struct {
	service     grpc.ServiceDesc
	opts        *options
	corsWrapper *cors.Cors
}

var (
	internalRequestHeadersWhitelist = []string{
		"U-A", // for gRPC-Web User Agent indicator.
	}
)

func Wrap(service grpc.ServiceDesc, options ...Option) *wrapper {
	opts := evaluateOptions(options)
	corsWrapper := cors.New(cors.Options{
		AllowOriginFunc:  opts.originFunc,
		AllowedHeaders:   append(opts.allowedRequestHeaders, internalRequestHeadersWhitelist...),
		ExposedHeaders:   nil,                                 // make sure that this is *nil*, otherwise the WebResponse overwrite will not work.
		AllowCredentials: true,                                // always allow credentials, otherwise :authorization headers won't work
		MaxAge:           int(10 * time.Minute / time.Second), // make sure pre-flights don't happen too often (every 5s for Chromium :( )
	})
	return &wrapper{
		service:     service,
		opts:        opts,
		corsWrapper: corsWrapper,
	}
}

// IsGrpcWebRequest determines if a request is a gRPC-Web request by checking that the "content-type" is
// "application/grpc-web" and that the method is POST.
func (w *wrapper) IsGrpcWebRequest(req *http.Request) bool {
	return req.Method == http.MethodPost && strings.HasPrefix(req.Header.Get("content-type"), "application/grpc-web")
}

// IsAcceptableGrpcCorsRequest determines if a request is a CORS pre-flight request for a gRPC-Web request and that this
// request is acceptable for CORS.
//
// You can control the CORS behaviour using `With*` options in the WrapServer function.
func (w *wrapper) IsAcceptableGrpcCorsRequest(req *http.Request) bool {
	accessControlHeaders := strings.ToLower(req.Header.Get("Access-Control-Request-Headers"))
	if req.Method == http.MethodOptions && strings.Contains(accessControlHeaders, "x-grpc-web") {
		if w.opts.corsForRegisteredEndpointsOnly {
			return w.isRequestForRegisteredEndpoint(req)
		}
		return true
	}
	return false
}

func (w *wrapper) isRequestForRegisteredEndpoint(req *http.Request) bool {
	registeredEndpoints := ListEndpoints(w.service)
	requestedEndpoint := req.URL.Path
	for _, v := range registeredEndpoints {
		if v == requestedEndpoint {
			return true
		}
	}
	return false
}

func hackIntoNormalGrpcRequest(req *http.Request) *http.Request {
	// Hack, this should be a shallow copy, but let's see if this works
	req.ProtoMajor = 2
	req.ProtoMinor = 0
	contentType := req.Header.Get("content-type")
	req.Header.Set("content-type", strings.Replace(contentType, "application/grpc-web", "application/grpc", 1))
	return req
}
