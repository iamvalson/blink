package service_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/api/service"
)

// Test MediaService validation rules
func TestMediaService_Validation(t *testing.T) {
	userID := uuid.New()

	t.Run("Unsupported MIME type", func(t *testing.T) {
		svc := service.NewMediaService(nil, nil)
		_, err := svc.Upload(context.Background(), service.UploadInput{
			UserID:           userID,
			OriginalFilename: "malicious.exe",
			ContentType:      "application/x-msdownload",
			Size:             100,
			Body:             strings.NewReader("bad"),
		})
		if !errors.Is(err, service.ErrUnsupportedMediaType) {
			t.Fatalf("expected ErrUnsupportedMediaType, got %v", err)
		}
	})

	t.Run("Empty file", func(t *testing.T) {
		svc := service.NewMediaService(nil, nil)
		_, err := svc.Upload(context.Background(), service.UploadInput{
			UserID:           userID,
			OriginalFilename: "empty.mp4",
			ContentType:      "video/mp4",
			Size:             0,
			Body:             bytes.NewReader([]byte{}),
		})
		if !errors.Is(err, service.ErrEmptyFile) {
			t.Fatalf("expected ErrEmptyFile, got %v", err)
		}
	})

	t.Run("Oversized video", func(t *testing.T) {
		svc := service.NewMediaService(nil, nil)
		_, err := svc.Upload(context.Background(), service.UploadInput{
			UserID:           userID,
			OriginalFilename: "huge.mp4",
			ContentType:      "video/mp4",
			Size:             service.MaxVideoSize + 1,
			Body:             strings.NewReader("data"),
		})
		if !errors.Is(err, service.ErrFileTooLarge) {
			t.Fatalf("expected ErrFileTooLarge, got %v", err)
		}
	})

	t.Run("Oversized image", func(t *testing.T) {
		svc := service.NewMediaService(nil, nil)
		_, err := svc.Upload(context.Background(), service.UploadInput{
			UserID:           userID,
			OriginalFilename: "huge.png",
			ContentType:      "image/png",
			Size:             service.MaxImageSize + 1,
			Body:             strings.NewReader("data"),
		})
		if !errors.Is(err, service.ErrFileTooLarge) {
			t.Fatalf("expected ErrFileTooLarge, got %v", err)
		}
	})
}

func TestMediaService_InitiateUploadValidation(t *testing.T) {
	svc := service.NewMediaService(nil, nil)
	userID := uuid.New()

	t.Run("unauthenticated", func(t *testing.T) {
		_, err := svc.InitiateUpload(context.Background(), service.InitiateUploadInput{
			UserID:      uuid.Nil,
			Filename:    "video.mp4",
			ContentType: "video/mp4",
			Size:        1000,
		})
		if !errors.Is(err, service.ErrMediaUnauthorized) {
			t.Fatalf("expected ErrMediaUnauthorized, got %v", err)
		}
	})

	t.Run("unsupported content type", func(t *testing.T) {
		_, err := svc.InitiateUpload(context.Background(), service.InitiateUploadInput{
			UserID:      userID,
			Filename:    "doc.pdf",
			ContentType: "application/pdf",
			Size:        1000,
		})
		if !errors.Is(err, service.ErrUnsupportedMediaType) {
			t.Fatalf("expected ErrUnsupportedMediaType, got %v", err)
		}
	})

	t.Run("zero size file", func(t *testing.T) {
		_, err := svc.InitiateUpload(context.Background(), service.InitiateUploadInput{
			UserID:      userID,
			Filename:    "video.mp4",
			ContentType: "video/mp4",
			Size:        0,
		})
		if !errors.Is(err, service.ErrEmptyFile) {
			t.Fatalf("expected ErrEmptyFile, got %v", err)
		}
	})

	t.Run("oversized file", func(t *testing.T) {
		_, err := svc.InitiateUpload(context.Background(), service.InitiateUploadInput{
			UserID:      userID,
			Filename:    "video.mp4",
			ContentType: "video/mp4",
			Size:        service.MaxVideoSize + 1,
		})
		if !errors.Is(err, service.ErrFileTooLarge) {
			t.Fatalf("expected ErrFileTooLarge, got %v", err)
		}
	})
}
