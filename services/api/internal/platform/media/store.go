package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/google/uuid"
)

const URLLifetime = 2 * time.Minute

var ErrStorageUnavailable = errors.New("private media storage unavailable")

type StoreConfig struct {
	Endpoint, Bucket, Region string
	AccessKey, SecretKey     string
}

type Object struct {
	TenantID, ID uuid.UUID
	Type         string
	Size         int
	Checksum     string // Canonical base64 SHA-256.
}

type SignedRequest struct {
	URL     string      `json:"url"`
	Headers http.Header `json:"headers,omitempty"`
}

type Store struct {
	client *s3.Client
	signer *s3.PresignClient
	bucket string
}

func NewStore(cfg StoreConfig) (*Store, error) {
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil ||
		endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" ||
		cfg.Bucket == "" || cfg.Region == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, ErrStorageUnavailable
	}
	ip := net.ParseIP(endpoint.Hostname())
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return nil, ErrStorageUnavailable
	}
	client := s3.New(s3.Options{
		Region: cfg.Region, BaseEndpoint: aws.String(endpoint.String()), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		HTTPClient: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return &Store{client: client, signer: s3.NewPresignClient(client), bucket: cfg.Bucket}, nil
}

func (o Object) key(prefix string) string {
	return prefix + "/" + o.TenantID.String() + "/" + o.ID.String()
}

func (o Object) valid() bool {
	checksum, err := base64.StdEncoding.DecodeString(o.Checksum)
	if o.TenantID == uuid.Nil || o.ID == uuid.Nil || o.Size < 1 || o.Size > MaxBytes ||
		err != nil || len(checksum) != sha256.Size || base64.StdEncoding.EncodeToString(checksum) != o.Checksum {
		return false
	}
	return o.Type == "application/pdf" || o.Type == "image/png" || o.Type == "image/jpeg"
}

func (s *Store) SignUpload(ctx context.Context, o Object) (SignedRequest, error) {
	if !o.valid() {
		return SignedRequest{}, ErrInvalidObject
	}
	signed, err := s.signer.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(o.key("quarantine")),
		ContentLength: aws.Int64(int64(o.Size)), ContentType: aws.String(o.Type), ChecksumSHA256: aws.String(o.Checksum),
	}, s3.WithPresignExpires(URLLifetime))
	if err != nil {
		return SignedRequest{}, ErrStorageUnavailable
	}
	return SignedRequest{signed.URL, signed.SignedHeader}, nil
}

// ScanAndPromote scans the exact bytes written to the ready prefix. The
// client receives upload access only to quarantine and cannot replace them.
func (s *Store) ScanAndPromote(ctx context.Context, o Object, scanner Scanner) error {
	if !o.valid() {
		return ErrInvalidObject
	}
	if scanner == nil {
		return ErrScannerUnavailable
	}
	data, err := s.readObject(ctx, o, "quarantine")
	if err != nil {
		return err
	}
	return s.scanAndStore(ctx, o, data, scanner)
}

// StoreScannedUpload validates and scans a bounded server-side upload before
// writing any bytes to a downloadable object.
func (s *Store) StoreScannedUpload(ctx context.Context, o Object, data []byte, scanner Scanner) error {
	if !o.valid() || Validate(data, o.Size, o.Type) != nil {
		return ErrInvalidObject
	}
	digest := sha256.Sum256(data)
	if base64.StdEncoding.EncodeToString(digest[:]) != o.Checksum {
		return ErrInvalidObject
	}
	if scanner == nil {
		return ErrScannerUnavailable
	}
	return s.scanAndStore(ctx, o, data, scanner)
}

func (s *Store) scanAndStore(ctx context.Context, o Object, data []byte, scanner Scanner) error {
	if err := scanner.Scan(ctx, data); err != nil {
		return err
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(o.key("ready")), Body: bytes.NewReader(data),
		ContentLength: aws.Int64(int64(o.Size)), ContentType: aws.String(o.Type),
		ChecksumSHA256: aws.String(o.Checksum), IfNoneMatch: aws.String("*"),
	})
	if err != nil {
		var status *smithyhttp.ResponseError
		if !errors.As(err, &status) || status.HTTPStatusCode() != http.StatusPreconditionFailed {
			return ErrStorageUnavailable
		}
		// A prior attempt may have written the immutable object before its
		// database transaction finished. Accept only the exact scanned bytes.
		if _, verifyErr := s.readObject(ctx, o, "ready"); verifyErr != nil {
			return ErrStorageUnavailable
		}
	}
	return nil
}

func (s *Store) readObject(ctx context.Context, o Object, prefix string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(o.key(prefix))})
	if err != nil {
		return nil, ErrStorageUnavailable
	}
	defer obj.Body.Close()
	data, err := io.ReadAll(io.LimitReader(obj.Body, int64(o.Size)+1))
	if err != nil {
		return nil, ErrStorageUnavailable
	}
	sum := sha256.Sum256(data)
	checksum := base64.StdEncoding.EncodeToString(sum[:])
	if Validate(data, o.Size, o.Type) != nil || subtle.ConstantTimeCompare([]byte(checksum), []byte(o.Checksum)) != 1 {
		return nil, ErrInvalidObject
	}

	return data, nil
}

// The caller must authorize enrollment and READY state before issuing the URL.
func (s *Store) SignDownload(ctx context.Context, tenantID, id uuid.UUID) (SignedRequest, error) {
	if tenantID == uuid.Nil || id == uuid.Nil {
		return SignedRequest{}, ErrInvalidObject
	}
	key := (Object{TenantID: tenantID, ID: id}).key("ready")
	signed, err := s.signer.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), ResponseContentDisposition: aws.String("attachment"),
		ResponseCacheControl: aws.String("private, no-store"),
	}, s3.WithPresignExpires(URLLifetime))
	if err != nil || !strings.EqualFold(signed.Method, http.MethodGet) {
		return SignedRequest{}, ErrStorageUnavailable
	}
	return SignedRequest{signed.URL, signed.SignedHeader}, nil
}
