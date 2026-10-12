package mediastore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// R2Config holds configuration for the Cloudflare R2 storage adapter.
type R2Config struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	Endpoint        string
	Region          string
}

// S3ClientAPI specifies the S3 client operations used by R2Store.
type S3ClientAPI interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// S3PresignAPI specifies the S3 presign operations used by R2Store.
type S3PresignAPI interface {
	PresignPutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

// R2Store implements Store backed by Cloudflare R2 via the S3-compatible API.
type R2Store struct {
	client        S3ClientAPI
	presignClient S3PresignAPI
	bucket        string
}

// NewR2Store initializes an R2Store with the provided configuration.
func NewR2Store(cfg R2Config) (*R2Store, error) {
	if cfg.AccessKeyID == "" {
		return nil, errors.New("R2_ACCESS_KEY_ID is required for R2 storage")
	}
	if cfg.SecretAccessKey == "" {
		return nil, errors.New("R2_SECRET_ACCESS_KEY is required for R2 storage")
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		if cfg.AccountID == "" {
			return nil, errors.New("either R2_ACCOUNT_ID or R2_ENDPOINT is required for R2 storage")
		}
		endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)
	}

	bucket := cfg.BucketName
	if bucket == "" {
		bucket = "blink-media"
	}

	region := cfg.Region
	if region == "" {
		region = "auto"
	}

	creds := credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")
	s3Client := s3.New(s3.Options{
		Region:       region,
		Credentials:  creds,
		BaseEndpoint: aws.String(endpoint),
		UsePathStyle: true,
	})

	presignClient := s3.NewPresignClient(s3Client)

	return &R2Store{
		client:        s3Client,
		presignClient: presignClient,
		bucket:        bucket,
	}, nil
}

// NewR2StoreWithClient initializes an R2Store with injected S3 and presign clients (for testing).
func NewR2StoreWithClient(client S3ClientAPI, presignClient S3PresignAPI, bucket string) *R2Store {
	if bucket == "" {
		bucket = "blink-media"
	}
	return &R2Store{
		client:        client,
		presignClient: presignClient,
		bucket:        bucket,
	}
}

// Put writes an object directly to R2.
func (s *R2Store) Put(ctx context.Context, key string, r io.Reader, contentType string) (int64, error) {
	var body io.Reader = r
	var size int64 = -1

	// If reader is a bytes.Buffer or seeker, determine size if available.
	if buf, ok := r.(*bytes.Buffer); ok {
		size = int64(buf.Len())
	}

	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}

	_, err := s.client.PutObject(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("put object %q to R2: %w", key, err)
	}

	// If size was not known upfront, retrieve metadata
	if size < 0 {
		info, headErr := s.Head(ctx, key)
		if headErr == nil {
			return info.Size, nil
		}
		return 0, nil
	}

	return size, nil
}

// Get opens an object from R2 for streaming.
func (s *R2Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundError(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get object %q from R2: %w", key, err)
	}
	return out.Body, nil
}

// Delete removes an object from R2.
func (s *R2Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("delete object %q from R2: %w", key, err)
	}
	return nil
}

// Head checks object existence and retrieves metadata from R2.
func (s *R2Store) Head(ctx context.Context, key string) (*ObjectInfo, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundError(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("head object %q from R2: %w", key, err)
	}

	info := &ObjectInfo{
		Key: key,
	}
	if out.ContentLength != nil {
		info.Size = *out.ContentLength
	}
	if out.ContentType != nil {
		info.ContentType = *out.ContentType
	}
	if out.ETag != nil {
		info.ETag = *out.ETag
	}
	if out.LastModified != nil {
		info.LastModified = *out.LastModified
	}

	return info, nil
}

// PresignPut generates a short-lived presigned PUT URL for direct browser-to-R2 upload.
func (s *R2Store) PresignPut(ctx context.Context, key string, contentType string, size int64, ttl time.Duration) (*PresignedUpload, error) {
	if s.presignClient == nil {
		return nil, ErrPresignNotSupported
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}
	if size > 0 {
		input.ContentLength = aws.Int64(size)
	}

	presigned, err := s.presignClient.PresignPutObject(ctx, input, s3.WithPresignExpires(ttl))
	if err != nil {
		return nil, fmt.Errorf("presign put object %q: %w", key, err)
	}

	headers := make(map[string]string)
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	for k, v := range presigned.SignedHeader {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	return &PresignedUpload{
		URL:       presigned.URL,
		Method:    http.MethodPut,
		Headers:   headers,
		ExpiresAt: time.Now().Add(ttl),
	}, nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var notFound *s3types.NotFound
	var noSuchKey *s3types.NoSuchKey
	if errors.As(err, &notFound) || errors.As(err, &noSuchKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "NotFound") || strings.Contains(msg, "NoSuchKey") || strings.Contains(msg, "404")
}

