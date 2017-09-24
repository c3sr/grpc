package grpc

import (
	"io"
	"io/ioutil"
	"sync"

	"github.com/golang/snappy"
)

// NB: The grpc.{Compressor,Decompressor} implementations need to be goroutine
// safe as multiple goroutines may be using the same compressor/decompressor
// for different streams on the same connection.
var snappyWriterPool sync.Pool
var snappyReaderPool sync.Pool

type snappyCompressor struct {
}

func (snappyCompressor) Do(w io.Writer, p []byte) error {
	z, ok := snappyWriterPool.Get().(*snappy.Writer)
	if !ok {
		z = snappy.NewBufferedWriter(w)
	} else {
		z.Reset(w)
	}
	_, err := z.Write(p)
	if err == nil {
		err = z.Flush()
	}
	snappyWriterPool.Put(z)
	return err
}

func (snappyCompressor) Type() string {
	return "snappy"
}

type snappyDecompressor struct {
}

func (snappyDecompressor) Do(r io.Reader) ([]byte, error) {
	z, ok := snappyReaderPool.Get().(*snappy.Reader)
	if !ok {
		z = snappy.NewReader(r)
	} else {
		z.Reset(r)
	}
	b, err := ioutil.ReadAll(z)
	snappyReaderPool.Put(z)
	return b, err
}

func (snappyDecompressor) Type() string {
	return "snappy"
}
