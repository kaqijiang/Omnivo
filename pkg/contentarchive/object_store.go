package contentarchive

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

const objectOperationTimeout = 15 * time.Second

type s3Store struct {
	client *s3.Client
	bucket string
}

// NewS3Store uses explicit archive credentials, never gateway/provider keys or
// implicit instance credentials. The bucket must already exist and be private.
func NewS3Store(cfg Config) (ObjectStore, error) {
	if cfg.S3Bucket == "" || len(cfg.S3Bucket) > 63 || strings.ContainsAny(cfg.S3Bucket, "/\\\r\n\x00") {
		return nil, errors.New("invalid content archive bucket")
	}
	if cfg.S3AccessKey == "" || cfg.S3SecretKey == "" || cfg.S3Region == "" {
		return nil, errors.New("content archive object storage credentials and region are required")
	}
	if cfg.S3Endpoint != "" {
		endpoint, err := url.Parse(cfg.S3Endpoint)
		if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || (endpoint.Path != "" && endpoint.Path != "/") {
			return nil, errors.New("invalid content archive object storage endpoint")
		}
	}
	client := s3.NewFromConfig(aws.Config{
		Region:           cfg.S3Region,
		Credentials:      credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		HTTPClient:       &http.Client{Timeout: objectOperationTimeout},
		RetryMaxAttempts: 2,
	}, func(options *s3.Options) {
		options.UsePathStyle = cfg.S3PathStyle
		if cfg.S3Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.S3Endpoint)
		}
	})
	return &s3Store{client: client, bucket: cfg.S3Bucket}, nil
}

func (s *s3Store) Put(ctx context.Context, key string, data []byte) error {
	if !validObjectKey(key) || len(data) == 0 || len(data) > 40<<20 {
		return errors.New("invalid content archive object")
	}
	ctx, cancel := context.WithTimeout(ctx, objectOperationTimeout)
	defer cancel()
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))), ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return errors.New("content archive object write failed")
	}
	return nil
}

func (s *s3Store) Get(ctx context.Context, key string, maxBytes int64) ([]byte, error) {
	if !validObjectKey(key) || maxBytes <= 0 || maxBytes > 40<<20 {
		return nil, errors.New("invalid content archive object read")
	}
	ctx, cancel := context.WithTimeout(ctx, objectOperationTimeout)
	defer cancel()
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, errors.New("content archive object read failed")
	}
	defer result.Body.Close()
	if result.ContentLength != nil && *result.ContentLength > maxBytes {
		return nil, errors.New("content archive object exceeds read limit")
	}
	data, err := io.ReadAll(io.LimitReader(result.Body, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		return nil, errors.New("content archive object exceeds read limit or could not be read")
	}
	return data, nil
}

func (s *s3Store) Delete(ctx context.Context, key string) error {
	if !validObjectKey(key) {
		return errors.New("invalid content archive object key")
	}
	ctx, cancel := context.WithTimeout(ctx, objectOperationTimeout)
	defer cancel()
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		var apiError smithy.APIError
		if errors.As(err, &apiError) && apiError.ErrorCode() == "NoSuchKey" {
			return nil
		}
		return errors.New("content archive object deletion failed")
	}
	return nil
}

func validObjectKey(key string) bool {
	if !strings.HasPrefix(key, "content/v1/") || len(key) > 240 || strings.Contains(key, "..") {
		return false
	}
	for _, character := range key {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '/' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}
