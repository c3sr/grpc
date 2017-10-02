package grpc

import (
	"github.com/k0kubun/pp"
	"github.com/rai-project/config"
	"github.com/rai-project/vipertags"
)

type grpcConfig struct {
	EnableCUPTI *bool         `json:"cupti" config:"grpc.cupti"`
	done        chan struct{} `json:"-" config:"-"`
}

var (
	Config = &grpcConfig{
		done: make(chan struct{}),
	}
)

func (grpcConfig) ConfigName() string {
	return "grpc"
}

func (a *grpcConfig) SetDefaults() {
	vipertags.SetDefaults(a)
}

func (a *grpcConfig) Read() {
	defer close(a.done)
	vipertags.Fill(a)
	if a.EnableCUPTI == nil {
		a.EnableCUPTI = new(bool)
		*a.EnableCUPTI = DefaultCUPTIEnabled
	}
}

func (c grpcConfig) Wait() {
	<-c.done
}

func (c grpcConfig) String() string {
	return pp.Sprintln(c)
}

func (c grpcConfig) Debug() {
	log.Debug("grpc Config = ", c)
}

func init() {
	config.Register(Config)
}
