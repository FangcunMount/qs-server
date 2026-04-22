package aliyunoss

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/FangcunMount/component-base/pkg/logger"
	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	alioss "github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

type publicObjectStore struct {
	client       *alioss.Client
	bucket       string
	cacheControl string
}

var _ objectstorageport.PublicObjectStore = (*publicObjectStore)(nil)

// NewPublicObjectStore creates an OSS-backed public object store.
func NewPublicObjectStore(opts *options.OSSOptions) (objectstorageport.PublicObjectStore, error) {
	if opts == nil {
		return nil, fmt.Errorf("oss options are required")
	}

	cfg := alioss.LoadDefaultConfig()
	if opts.Region != "" {
		cfg = cfg.WithRegion(opts.Region)
	}
	if opts.Endpoint != "" {
		cfg = cfg.WithEndpoint(opts.Endpoint)
	}
	if opts.ConnectTimeout > 0 {
		cfg = cfg.WithConnectTimeout(opts.ConnectTimeout)
	}
	if opts.ReadWriteTimeout > 0 {
		cfg = cfg.WithReadWriteTimeout(opts.ReadWriteTimeout)
	}
	if opts.RetryMaxAttempts > 0 {
		cfg = cfg.WithRetryMaxAttempts(opts.RetryMaxAttempts)
	}
	if opts.UseInternalEndpoint {
		cfg = cfg.WithUseInternalEndpoint(true)
	}
	if opts.UseCName {
		cfg = cfg.WithUseCName(true)
	}

	provider, err := buildCredentialsProvider(opts)
	if err != nil {
		return nil, err
	}
	cfg = cfg.WithCredentialsProvider(provider)

	return &publicObjectStore{
		client:       alioss.NewClient(cfg),
		bucket:       opts.Bucket,
		cacheControl: opts.CacheControl,
	}, nil
}

func buildCredentialsProvider(opts *options.OSSOptions) (credentials.CredentialsProvider, error) {
	if opts.AccessKeyID != "" && opts.AccessKeySecret != "" {
		if opts.SessionToken != "" {
			return credentials.NewStaticCredentialsProvider(opts.AccessKeyID, opts.AccessKeySecret, opts.SessionToken), nil
		}
		return credentials.NewStaticCredentialsProvider(opts.AccessKeyID, opts.AccessKeySecret), nil
	}
	provider := credentials.NewEnvironmentVariableCredentialsProvider()
	if _, err := provider.GetCredentials(context.Background()); err != nil {
		return nil, fmt.Errorf("load oss credentials from environment: %w", err)
	}
	return provider, nil
}

func normalizeObjectKey(key string) (string, error) {
	normalized := strings.Trim(strings.TrimSpace(key), "/")
	if normalized == "" {
		return "", fmt.Errorf("object key cannot be empty")
	}
	return normalized, nil
}

// Put uploads a QR code object to OSS.
func (s *publicObjectStore) Put(ctx context.Context, key string, contentType string, body []byte) error {
	objectKey, err := normalizeObjectKey(key)
	if err != nil {
		return err
	}

	req := &alioss.PutObjectRequest{
		Bucket:        alioss.Ptr(s.bucket),
		Key:           alioss.Ptr(objectKey),
		Body:          bytes.NewReader(body),
		ContentType:   alioss.Ptr(contentType),
		ContentLength: alioss.Ptr(int64(len(body))),
	}
	if s.cacheControl != "" {
		req.CacheControl = alioss.Ptr(s.cacheControl)
	}

	if _, err := s.client.PutObject(ctx, req); err != nil {
		logger.L(ctx).Errorw("upload qrcode to oss failed",
			"action", "upload_qrcode_oss",
			"bucket", s.bucket,
			"object_key", objectKey,
			"error", err.Error(),
		)
		return fmt.Errorf("upload object %q to oss: %w", objectKey, err)
	}

	return nil
}

// Get opens an object stream from OSS.
func (s *publicObjectStore) Get(ctx context.Context, key string) (*objectstorageport.ObjectReader, error) {
	objectKey, err := normalizeObjectKey(key)
	if err != nil {
		return nil, err
	}

	result, err := s.client.GetObject(ctx, &alioss.GetObjectRequest{
		Bucket: alioss.Ptr(s.bucket),
		Key:    alioss.Ptr(objectKey),
	})
	if err != nil {
		if isObjectNotFound(err) {
			return nil, objectstorageport.ErrObjectNotFound
		}
		logger.L(ctx).Errorw("open qrcode object from oss failed",
			"action", "get_qrcode_object_oss",
			"bucket", s.bucket,
			"object_key", objectKey,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("get object %q from oss: %w", objectKey, err)
	}

	return &objectstorageport.ObjectReader{
		Body:          result.Body,
		ContentType:   derefOrDefault(result.ContentType, "image/png"),
		ContentLength: result.ContentLength,
		CacheControl:  s.cacheControl,
	}, nil
}

func isObjectNotFound(err error) bool {
	var serviceErr *alioss.ServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.StatusCode == 404 || serviceErr.Code == "NoSuchKey" || serviceErr.Code == "NoSuchBucket"
	}
	return false
}

func derefOrDefault(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}
