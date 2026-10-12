package mediastore_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/iamvalson/blink/internal/config"
	"github.com/iamvalson/blink/internal/mediastore"
)

type mockS3Client struct {
	putFunc    func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	getFunc    func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	headFunc   func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	deleteFunc func(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putFunc != nil {
		return m.putFunc(ctx, params, optFns...)
	}
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, params, optFns...)
	}
	return &s3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader("sample-content")),
	}, nil
}

func (m *mockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if m.headFunc != nil {
		return m.headFunc(ctx, params, optFns...)
	}
	size := int64(1024)
	ct := "video/mp4"
	return &s3.HeadObjectOutput{
		ContentLength: &size,
		ContentType:   &ct,
	}, nil
}

func (m *mockS3Client) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, params, optFns...)
	}
	return &s3.DeleteObjectOutput{}, nil
}

type mockS3PresignClient struct {
	presignFunc func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

func (m *mockS3PresignClient) PresignPutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	if m.presignFunc != nil {
		return m.presignFunc(ctx, params, optFns...)
	}
	signedHeaders := http.Header{}
	if params.ContentType != nil {
		signedHeaders.Set("Content-Type", *params.ContentType)
	}
	return &v4.PresignedHTTPRequest{
		URL:          "https://blink-media.r2.cloudflarestorage.com/upload-target?X-Amz-Signature=test",
		Method:       "PUT",
		SignedHeader: signedHeaders,
	}, nil
}

func TestNewR2Store_Validation(t *testing.T) {
	t.Run("missing access key", func(t *testing.T) {
		_, err := mediastore.NewR2Store(mediastore.R2Config{
			SecretAccessKey: "secret",
			AccountID:       "acc",
		})
		if err == nil {
			t.Fatal("expected error for missing access key")
		}
	})

	t.Run("missing secret key", func(t *testing.T) {
		_, err := mediastore.NewR2Store(mediastore.R2Config{
			AccessKeyID: "key",
			AccountID:   "acc",
		})
		if err == nil {
			t.Fatal("expected error for missing secret key")
		}
	})

	t.Run("missing account and endpoint", func(t *testing.T) {
		_, err := mediastore.NewR2Store(mediastore.R2Config{
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
		})
		if err == nil {
			t.Fatal("expected error for missing account and endpoint")
		}
	})

	t.Run("valid configuration", func(t *testing.T) {
		store, err := mediastore.NewR2Store(mediastore.R2Config{
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
			AccountID:       "acc",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if store == nil {
			t.Fatal("expected non-nil store")
		}
	})
}

func TestR2Store_PresignPut(t *testing.T) {
	mockS3 := &mockS3Client{}
	mockPresign := &mockS3PresignClient{}
	store := mediastore.NewR2StoreWithClient(mockS3, mockPresign, "blink-media")

	res, err := store.PresignPut(context.Background(), "media/123/video.mp4", "video/mp4", 1024, 15*time.Minute)
	if err != nil {
		t.Fatalf("PresignPut error = %v", err)
	}
	if !strings.Contains(res.URL, "upload-target") {
		t.Fatalf("expected presigned URL to contain upload-target, got %s", res.URL)
	}
	if res.Method != "PUT" {
		t.Fatalf("expected method PUT, got %s", res.Method)
	}
	if res.Headers["Content-Type"] != "video/mp4" {
		t.Fatalf("expected Content-Type header video/mp4, got %v", res.Headers)
	}
	if time.Until(res.ExpiresAt) <= 0 {
		t.Fatal("expected ExpiresAt to be in the future")
	}
}

func TestR2Store_Head(t *testing.T) {
	t.Run("object found", func(t *testing.T) {
		mockS3 := &mockS3Client{}
		store := mediastore.NewR2StoreWithClient(mockS3, nil, "blink-media")

		info, err := store.Head(context.Background(), "test.mp4")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Size != 1024 {
			t.Fatalf("expected size 1024, got %d", info.Size)
		}
		if info.ContentType != "video/mp4" {
			t.Fatalf("expected contentType video/mp4, got %s", info.ContentType)
		}
	})

	t.Run("object not found", func(t *testing.T) {
		mockS3 := &mockS3Client{
			headFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
				return nil, &s3types.NotFound{}
			},
		}
		store := mediastore.NewR2StoreWithClient(mockS3, nil, "blink-media")

		_, err := store.Head(context.Background(), "missing.mp4")
		if !errors.Is(err, mediastore.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestR2Store_GetAndDelete(t *testing.T) {
	mockS3 := &mockS3Client{
		getFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader("video-payload-data")),
			}, nil
		},
	}
	store := mediastore.NewR2StoreWithClient(mockS3, nil, "blink-media")

	rc, err := store.Get(context.Background(), "video.mp4")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "video-payload-data" {
		t.Fatalf("unexpected data: %s", string(data))
	}

	if err := store.Delete(context.Background(), "video.mp4"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
}

func TestInitStore_ProductionVsDevelopment(t *testing.T) {
	t.Run("production rejects local", func(t *testing.T) {
		cfg := &config.Config{
			Env:                "production",
			MediaStorageDriver: "local",
		}
		_, err := mediastore.InitStore(cfg)
		if err == nil {
			t.Fatal("expected production to reject local driver, got nil")
		}
	})

	t.Run("development allows local", func(t *testing.T) {
		cfg := &config.Config{
			Env:                "development",
			MediaStorageDriver: "local",
			MediaStoragePath:   t.TempDir(),
		}
		store, err := mediastore.InitStore(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if store == nil {
			t.Fatal("expected store")
		}
	})

	t.Run("r2 initialized with config", func(t *testing.T) {
		cfg := &config.Config{
			Env:                "production",
			MediaStorageDriver: "r2",
			R2AccountID:        "acc-1",
			R2AccessKeyID:      "key-1",
			R2SecretAccessKey:  "sec-1",
			R2BucketName:       "blink-media",
		}
		store, err := mediastore.InitStore(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if store == nil {
			t.Fatal("expected store")
		}
	})
}

func TestLocalStore_Operations(t *testing.T) {
	dir := t.TempDir()
	store, err := mediastore.NewLocalStore(dir)
	if err != nil {
		t.Fatalf("NewLocalStore error: %v", err)
	}

	ctx := context.Background()
	key := "test/file.txt"
	content := []byte("hello world")

	// Put
	n, err := store.Put(ctx, key, bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("Put error: %v", err)
	}
	if n != int64(len(content)) {
		t.Fatalf("expected %d bytes written, got %d", len(content), n)
	}

	// Head
	info, err := store.Head(ctx, key)
	if err != nil {
		t.Fatalf("Head error: %v", err)
	}
	if info.Size != int64(len(content)) {
		t.Fatalf("expected size %d, got %d", len(content), info.Size)
	}

	// Get
	rc, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	readBytes, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(readBytes, content) {
		t.Fatalf("expected content %q, got %q", content, readBytes)
	}

	// Delete
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	// Head missing
	_, err = store.Head(ctx, key)
	if !errors.Is(err, mediastore.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

