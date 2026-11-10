package mediastore

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (s *R2Store) PresignRead(ctx context.Context, key string, ttl time.Duration) (string, error) {
	client, ok := s.presignClient.(interface {
		PresignGetObject(context.Context, *s3.GetObjectInput, ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
	})
	if !ok {
		return "", ErrPresignNotSupported
	}
	request, err := client.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", errors.New("failed to authorize Instagram media delivery")
	}
	return request.URL, nil
}
