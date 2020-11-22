package grpc

import (
	"runtime"

	"github.com/c3sr/config"
	"github.com/c3sr/nvidia-smi"
)

var (
	DefaultCUPTIEnabled bool
)

func init() {
	config.BeforeInit(func() {
		nvidiasmi.Wait()
		DefaultCUPTIEnabled = (runtime.GOOS == "linux") && nvidiasmi.HasGPU
	})
}
