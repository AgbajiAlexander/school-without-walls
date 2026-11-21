package media

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

type scanFunc func(context.Context, []byte) error

func (f scanFunc) Scan(ctx context.Context, b []byte) error { return f(ctx, b) }

func TestPrivateMediaPromotion(t *testing.T) {
	data := []byte("%PDF-1.4\nfictional media fixture")
	sum := sha256.Sum256(data)
	o := Object{TenantID: uuid.New(), ID: uuid.New(), Type: "application/pdf", Size: len(data), Checksum: base64.StdEncoding.EncodeToString(sum[:])}
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && (r.URL.Path == "/fixture/"+o.key("quarantine") || (writes.Load() > 0 && r.URL.Path == "/fixture/"+o.key("ready"))):
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			_, _ = w.Write(data)
		case r.Method == "PUT" && r.URL.Path == "/fixture/"+o.key("ready"):
			first := writes.Add(1) == 1
			body, err := io.ReadAll(r.Body)
			if err != nil || !bytes.Equal(body, data) || r.Header.Get("If-None-Match") != "*" {
				t.Error("promotion did not preserve scanned data and immutable write")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if !first {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	store, err := NewStore(StoreConfig{Endpoint: server.URL, Bucket: "fixture", Region: "us-east-1", AccessKey: "fictional", SecretKey: "fictional"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	upload, err := store.SignUpload(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(upload.URL)
	if err != nil || u.Path != "/fixture/"+o.key("quarantine") || u.Query().Get("X-Amz-Expires") != "120" ||
		(u.Query().Get("X-Amz-Checksum-Sha256") != o.Checksum && upload.Headers.Get("X-Amz-Checksum-Sha256") != o.Checksum) {
		t.Fatal("upload grant omitted its private prefix, checksum or expiry")
	}
	if !strings.Contains(u.Query().Get("X-Amz-SignedHeaders"), "content-length") {
		t.Fatal("upload grant does not bind its declared payload size")
	}
	download, err := store.SignDownload(ctx, o.TenantID, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	u, err = url.Parse(download.URL)
	if err != nil || u.Path != "/fixture/"+o.key("ready") || u.Query().Get("X-Amz-Expires") != "120" ||
		u.Query().Get("response-content-disposition") != "attachment" {
		t.Fatal("download grant omitted its ready prefix, expiry or attachment disposition")
	}
	unavailable := scanFunc(func(context.Context, []byte) error { return ErrScannerUnavailable })
	if err := store.ScanAndPromote(ctx, o, unavailable); !errors.Is(err, ErrScannerUnavailable) || writes.Load() != 0 {
		t.Fatal("scanner failure must leave the object quarantined")
	}
	wrong := o
	wrong.Type = "image/png"
	if err := store.ScanAndPromote(ctx, wrong, unavailable); !errors.Is(err, ErrInvalidObject) || writes.Load() != 0 {
		t.Fatal("invalid content type must never reach promotion")
	}
	if err := store.ScanAndPromote(ctx, o, scanFunc(func(_ context.Context, actual []byte) error {
		if !bytes.Equal(actual, data) {
			t.Error("scanner received different bytes")
		}
		return nil
	})); err != nil || writes.Load() != 1 {
		t.Fatalf("clean promotion failed: %v", err)
	}
	if err := store.ScanAndPromote(ctx, o, scanFunc(func(context.Context, []byte) error { return nil })); err != nil || writes.Load() != 2 {
		t.Fatal("retry must confirm the existing immutable object")
	}

}

func TestClamScanner(t *testing.T) {
	for _, test := range []struct {
		name, reply string
		want        error
	}{
		{"clean", "stream: OK\x00", nil},
		{"infected", "stream: Fixture FOUND\x00", ErrInfected},
		{"interrupted", "stream: OK", ErrScannerUnavailable},
		{"scanner error", "stream: internal error ERROR\x00", ErrScannerUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				read := bufio.NewReader(conn)
				command, err := read.ReadString(0)
				if err != nil || command != "zINSTREAM\x00" {
					done <- errors.New("invalid scan command")
					return
				}
				var size uint32
				if err := binary.Read(read, binary.BigEndian, &size); err != nil || size != 3 {
					done <- errors.New("invalid scan size")
					return
				}
				data := make([]byte, 3)
				if _, err := io.ReadFull(read, data); err != nil || string(data) != "abc" {
					done <- errors.New("invalid scan body")
					return
				}
				if err := binary.Read(read, binary.BigEndian, &size); err != nil || size != 0 {
					done <- errors.New("scan stream was not terminated")
					return
				}
				_, err = io.WriteString(conn, test.reply)
				done <- err
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err = (ClamScanner{Network: "tcp", Address: listener.Addr().String()}).Scan(ctx, []byte("abc"))
			if !errors.Is(err, test.want) {
				t.Errorf("got %v, want %v", err, test.want)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
