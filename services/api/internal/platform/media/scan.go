package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

const MaxBytes = 8 << 20

var (
	ErrInvalidObject      = errors.New("invalid media size or type")
	ErrInfected           = errors.New("media reported as infected")
	ErrScannerUnavailable = errors.New("media scanner unavailable")
)

type Scanner interface {
	Scan(context.Context, []byte) error
}

func Validate(data []byte, expectedBytes int, expectedType string) error {
	if expectedBytes < 1 || expectedBytes > MaxBytes || len(data) != expectedBytes {
		return ErrInvalidObject
	}
	switch expectedType {
	case "application/pdf", "image/png", "image/jpeg":
		if http.DetectContentType(data) == expectedType {
			return nil
		}
	}
	return ErrInvalidObject
}

// ClamScanner speaks INSTREAM to a local or explicitly private scanner.
type ClamScanner struct {
	Network string
	Address string
}

func (s ClamScanner) Scan(ctx context.Context, data []byte) error {
	if len(data) == 0 || len(data) > MaxBytes {
		return ErrInvalidObject
	}
	if (s.Network != "unix" && s.Network != "tcp") || s.Address == "" {
		return ErrScannerUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, s.Network, s.Address)
	if err != nil {
		return ErrScannerUnavailable
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if conn.SetDeadline(deadline) != nil {
		return ErrScannerUnavailable
	}
	out := bufio.NewWriter(conn)
	if _, err := out.WriteString("zINSTREAM\x00"); err != nil {
		return ErrScannerUnavailable
	}
	reader := bytes.NewReader(data)
	var length [4]byte
	var chunk [16 << 10]byte
	for reader.Len() > 0 {
		n, _ := reader.Read(chunk[:])
		binary.BigEndian.PutUint32(length[:], uint32(n))
		if _, err := out.Write(length[:]); err != nil {
			return ErrScannerUnavailable
		}
		if _, err := out.Write(chunk[:n]); err != nil {
			return ErrScannerUnavailable
		}
	}
	if _, err := out.Write([]byte{0, 0, 0, 0}); err != nil || out.Flush() != nil {
		return ErrScannerUnavailable
	}
	reply, err := bufio.NewReaderSize(io.LimitReader(conn, 1024), 1024).ReadSlice(0)
	if err != nil {
		return ErrScannerUnavailable
	}
	reply = bytes.TrimSuffix(reply, []byte{0})
	switch {
	case bytes.Equal(reply, []byte("stream: OK")):
		return nil
	case bytes.HasPrefix(reply, []byte("stream: ")) && bytes.HasSuffix(reply, []byte(" FOUND")):
		return ErrInfected
	default:
		return ErrScannerUnavailable
	}
}
