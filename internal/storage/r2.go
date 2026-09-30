package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type R2Uploader struct {
	client *s3.Client
	bucket string
	prefix string
}

func NewR2Uploader(endpoint, bucket, prefix, accessKeyID, secretAccessKey string) (*R2Uploader, error) {
	for name, value := range map[string]string{
		"endpoint":        endpoint,
		"bucket":          bucket,
		"access key ID":   accessKeyID,
		"secret access key": secretAccessKey,
	} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("R2 %s is required", name)
		}
	}

	config := aws.Config{
		Region:      "auto",
		Credentials: credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	}
	client := s3.NewFromConfig(config, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	return &R2Uploader{client: client, bucket: bucket, prefix: strings.Trim(prefix, "/")}, nil
}

func (uploader *R2Uploader) Upload(ctx context.Context, objectName string, body io.Reader) error {
	key := objectName
	if uploader.prefix != "" {
		key = uploader.prefix + "/" + objectName
	}
	_, err := uploader.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(uploader.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String("application/gzip"),
	})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}