package storage

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3ClientFactory 负责创建 S3 SDK client。
type S3ClientFactory struct{}

// NewS3ClientFactory 创建 S3 client 工厂。
func NewS3ClientFactory() *S3ClientFactory {
	return &S3ClientFactory{}
}

// New 根据配置创建 S3 client 与 presign client。
func (f *S3ClientFactory) New(ctx context.Context, cfg S3Config) (*awss3.Client, *awss3.PresignClient, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, nil, err
	}

	client := awss3.NewFromConfig(awsCfg, func(options *awss3.Options) {
		options.UsePathStyle = cfg.ForcePathStyle
		if cfg.Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})

	return client, awss3.NewPresignClient(client), nil
}
