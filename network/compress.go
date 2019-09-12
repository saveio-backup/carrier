/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-09-03
 */
package network

import (
	"bytes"
	"compress/gzip"
	"compress/lzw"
	"compress/zlib"
	"io"

	"github.com/pkg/errors"
)

type AlgoType uint16

const (
	NONE  = AlgoType(0)
	BZIP2 = AlgoType(1)
	GZIP  = AlgoType(2)
	ZLIB  = AlgoType(3)
	LZW   = AlgoType(4)
	FLAT  = AlgoType(5)
)

var AlgoName = map[AlgoType]string{
	NONE:  "unused algo",
	BZIP2: "BZIP2",
	GZIP:  "GZIP",
	ZLIB:  "ZLIB",
	LZW:   "LZW",
	FLAT:  "FLAT"}

type CompressCondition struct {
	Size int
}

func (n *Network) GenCompressInfo(msgSize int) uint16 {
	//in front 4-bit stand for enable compress or not;
	//end front 4-bit stand for algo type;
	var enable uint16
	if n.compressEnable && msgSize >= n.CompressCondition.Size {
		enable = 1
	} else {
		enable = 0
	}

	if n.compressAlgo > AlgoType(0xFF) {
		panic("compress algo value invalid")
	}

	return (enable << 8) | uint16(n.compressAlgo)
}

func (n *Network) GetCompressInfo(info uint16) (AlgoType, bool) {
	//in front 4-bit stand for enable compress or not;
	//end front 4-bit stand for algo type;
	var enable bool
	if info>>8 == 1 {
		enable = true
	}
	if info>>8 == 0 {
		enable = false
	}
	return AlgoType(info & 0xFF), enable
}

func (n *Network) Compress(message []byte) ([]byte, error) {
	switch n.compressAlgo {
	case BZIP2:
		return doBzip2Compress(message), nil
	case GZIP:
		return doGzipCompress(message), nil
	case ZLIB:
		return doZlibCompress(message), nil
	case LZW:
		return doLzwCompress(message), nil
	case NONE:
		return message, nil
	default:
		return nil, errors.New("compress algo type error")
	}
	return nil, errors.New("unknow err about compress algo")
}

func (n *Network) Uncompress(message []byte, algo AlgoType) ([]byte, error) {
	switch algo {
	case BZIP2:
		return doBzip2Uncompress(message), nil
	case GZIP:
		return doGzipUncompress(message), nil
	case ZLIB:
		return doZlibUnCompress(message), nil
	case LZW:
		return doLzwUncompress(message), nil

	default:
		return message, nil
	}
	return nil, nil
}

func doLzwCompress(src []byte) []byte {
	var in bytes.Buffer
	w := lzw.NewWriter(&in, lzw.MSB, 8)
	w.Write(src)
	w.Close()
	return in.Bytes()
}

func doLzwUncompress(compressSrc []byte) []byte {
	b := bytes.NewReader(compressSrc)
	var out bytes.Buffer
	r := lzw.NewReader(b, lzw.MSB, 8)
	io.Copy(&out, r)
	return out.Bytes()
}

func doGzipCompress(src []byte) []byte {
	var in bytes.Buffer
	w := gzip.NewWriter(&in)
	w.Write(src)
	w.Close()
	return in.Bytes()
}

func doGzipUncompress(compressSrc []byte) []byte {
	b := bytes.NewReader(compressSrc)
	var out bytes.Buffer
	r, _ := gzip.NewReader(b)
	io.Copy(&out, r)
	return out.Bytes()
}

func doBzip2Compress(src []byte) []byte {
	return src
}

func doBzip2Uncompress(src []byte) []byte {
	return src
}

func doZlibCompress(src []byte) []byte {
	var in bytes.Buffer
	w := zlib.NewWriter(&in)
	w.Write(src)
	w.Close()
	return in.Bytes()
}

func doZlibUnCompress(compressSrc []byte) []byte {
	b := bytes.NewReader(compressSrc)
	var out bytes.Buffer
	r, _ := zlib.NewReader(b)
	io.Copy(&out, r)
	return out.Bytes()
}
