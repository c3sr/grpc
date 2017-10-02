package grpc

import (
	"runtime"

	"github.com/rai-project/config"
	"github.com/rai-project/nvidia-smi"
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
