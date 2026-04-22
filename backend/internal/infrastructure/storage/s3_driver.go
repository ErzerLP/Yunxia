package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awss3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"yunxia/internal/domain/entity"
	domainstorage "yunxia/internal/domain/storage"
)

// S3Driver 提供 S3 存储源的探测、浏览与预签名下载能力。
type S3Driver struct {
	clientFactory *S3ClientFactory
}

// NewS3Driver 创建 S3 驱动。
func NewS3Driver(clientFactory *S3ClientFactory) *S3Driver {
	if clientFactory == nil {
		clientFactory = NewS3ClientFactory()
	}
	return &S3Driver{clientFactory: clientFactory}
}

// Test 检查 bucket 是否可访问。
func (d *S3Driver) Test(ctx context.Context, source *entity.StorageSource) error {
	cfg, err := ParseS3ConfigJSON(source.ConfigJSON)
	if err != nil {
		return err
	}
	client, _, err := d.clientFactory.New(ctx, cfg)
	if err != nil {
		return err
	}
	_, err = client.HeadBucket(ctx, &awss3.HeadBucketInput{
		Bucket: aws.String(cfg.Bucket),
	})
	return err
}

// List 列出指定目录下的对象。
func (d *S3Driver) List(ctx context.Context, source *entity.StorageSource, virtualPath string) ([]domainstorage.StorageEntry, error) {
	cfg, err := ParseS3ConfigJSON(source.ConfigJSON)
	if err != nil {
		return nil, err
	}
	prefix, err := buildS3ListPrefix(cfg, source.RootPath, virtualPath)
	if err != nil {
		return nil, err
	}
	client, _, err := d.clientFactory.New(ctx, cfg)
	if err != nil {
		return nil, err
	}

	output, err := client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(cfg.Bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return nil, err
	}

	items := make([]domainstorage.StorageEntry, 0, len(output.CommonPrefixes)+len(output.Contents))
	for _, commonPrefix := range output.CommonPrefixes {
		childName := nameFromChildPrefix(prefix, aws.ToString(commonPrefix.Prefix))
		if childName == "" {
			continue
		}
		items = append(items, domainstorage.StorageEntry{
			Name:       childName,
			Path:       joinVirtualPath(virtualPath, childName),
			IsDir:      true,
			ModifiedAt: time.Unix(0, 0).UTC(),
		})
	}

	for _, object := range output.Contents {
		key := aws.ToString(object.Key)
		if key == prefix {
			continue
		}
		childName := strings.TrimPrefix(key, prefix)
		if childName == "" || strings.Contains(childName, "/") {
			continue
		}
		modifiedAt := aws.ToTime(object.LastModified)
		if modifiedAt.IsZero() {
			modifiedAt = time.Unix(0, 0).UTC()
		}
		items = append(items, domainstorage.StorageEntry{
			Name:       childName,
			Path:       joinVirtualPath(virtualPath, childName),
			IsDir:      false,
			Size:       aws.ToInt64(object.Size),
			ETag:       strings.Trim(aws.ToString(object.ETag), `"`),
			ModifiedAt: modifiedAt,
		})
	}

	return items, nil
}

// SearchByName 按名称搜索对象。
func (d *S3Driver) SearchByName(ctx context.Context, source *entity.StorageSource, pathPrefix, keyword string) ([]domainstorage.StorageEntry, error) {
	cfg, err := ParseS3ConfigJSON(source.ConfigJSON)
	if err != nil {
		return nil, err
	}
	prefix, err := buildS3SearchPrefix(cfg, source.RootPath, pathPrefix)
	if err != nil {
		return nil, err
	}
	client, _, err := d.clientFactory.New(ctx, cfg)
	if err != nil {
		return nil, err
	}

	paginator := awss3.NewListObjectsV2Paginator(client, &awss3.ListObjectsV2Input{
		Bucket: aws.String(cfg.Bucket),
		Prefix: aws.String(prefix),
	})

	lowerKeyword := strings.ToLower(keyword)
	items := make([]domainstorage.StorageEntry, 0)
	for paginator.HasMorePages() {
		page, pageErr := paginator.NextPage(ctx)
		if pageErr != nil {
			return nil, pageErr
		}
		for _, object := range page.Contents {
			key := aws.ToString(object.Key)
			if key == "" || strings.HasSuffix(key, "/") {
				continue
			}
			name := path.Base(key)
			if !strings.Contains(strings.ToLower(name), lowerKeyword) {
				continue
			}
			virtualPath, virtualErr := keyToVirtualPath(cfg, source.RootPath, key)
			if virtualErr != nil {
				continue
			}
			modifiedAt := aws.ToTime(object.LastModified)
			if modifiedAt.IsZero() {
				modifiedAt = time.Unix(0, 0).UTC()
			}
			items = append(items, domainstorage.StorageEntry{
				Name:       name,
				Path:       virtualPath,
				IsDir:      false,
				Size:       aws.ToInt64(object.Size),
				ETag:       strings.Trim(aws.ToString(object.ETag), `"`),
				ModifiedAt: modifiedAt,
			})
		}
	}

	return items, nil
}

// Stat 返回单个对象或伪目录信息。
func (d *S3Driver) Stat(ctx context.Context, source *entity.StorageSource, virtualPath string) (*domainstorage.StorageEntry, error) {
	info, err := d.statPath(ctx, source, virtualPath)
	if err != nil {
		return nil, err
	}
	entry := info.entry
	return &entry, nil
}

// Mkdir 在 S3 中通过目录标记对象创建空目录。
func (d *S3Driver) Mkdir(ctx context.Context, source *entity.StorageSource, parentPath, name string) (*domainstorage.StorageEntry, error) {
	cfg, err := ParseS3ConfigJSON(source.ConfigJSON)
	if err != nil {
		return nil, err
	}
	client, _, err := d.clientFactory.New(ctx, cfg)
	if err != nil {
		return nil, err
	}

	isDir, err := d.isDirectoryWithClient(ctx, client, cfg, source.RootPath, parentPath)
	if err != nil {
		return nil, err
	}
	if !isDir {
		return nil, os.ErrInvalid
	}

	targetPath := joinVirtualPath(parentPath, name)
	targetInfo, err := d.tryStatPathWithClient(ctx, client, cfg, source.RootPath, targetPath)
	if err != nil {
		return nil, err
	}
	if targetInfo != nil {
		return nil, fs.ErrExist
	}

	markerKey := prefixFromJoinedPath(cfg, path.Join(source.RootPath, targetPath))
	_, err = client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(markerKey),
		Body:   strings.NewReader(""),
	})
	if err != nil {
		return nil, err
	}

	return &domainstorage.StorageEntry{
		Name:       name,
		Path:       targetPath,
		IsDir:      true,
		ModifiedAt: time.Now(),
	}, nil
}

// Rename 在同目录下重命名对象或伪目录。
func (d *S3Driver) Rename(ctx context.Context, source *entity.StorageSource, virtualPath, newName string) (*domainstorage.StorageEntry, error) {
	if virtualPath == "/" {
		return nil, os.ErrInvalid
	}
	parentVirtual := path.Dir(virtualPath)
	if parentVirtual == "." {
		parentVirtual = "/"
	}
	newVirtual := joinVirtualPath(parentVirtual, newName)
	if newVirtual == virtualPath {
		return nil, fs.ErrExist
	}

	return d.renameOrCopy(ctx, source, virtualPath, newVirtual, true)
}

// Move 将对象或伪目录移动到目标目录。
func (d *S3Driver) Move(ctx context.Context, source *entity.StorageSource, virtualPath, targetPath string) error {
	if virtualPath == "/" {
		return os.ErrInvalid
	}
	entry, err := d.statPath(ctx, source, virtualPath)
	if err != nil {
		return err
	}
	if !entry.isDir && targetPath == virtualPath {
		return os.ErrInvalid
	}
	isDir, err := d.isDirectory(ctx, source, targetPath)
	if err != nil {
		return err
	}
	if !isDir {
		return os.ErrInvalid
	}

	newVirtual := joinVirtualPath(targetPath, path.Base(virtualPath))
	if newVirtual == virtualPath {
		return fs.ErrExist
	}
	_, err = d.renameOrCopy(ctx, source, virtualPath, newVirtual, true)
	return err
}

// Copy 复制对象或伪目录到目标目录。
func (d *S3Driver) Copy(ctx context.Context, source *entity.StorageSource, virtualPath, targetPath string) error {
	if virtualPath == "/" {
		return os.ErrInvalid
	}
	isDir, err := d.isDirectory(ctx, source, targetPath)
	if err != nil {
		return err
	}
	if !isDir {
		return os.ErrInvalid
	}

	newVirtual := joinVirtualPath(targetPath, path.Base(virtualPath))
	if newVirtual == virtualPath {
		return fs.ErrExist
	}
	_, err = d.renameOrCopy(ctx, source, virtualPath, newVirtual, false)
	return err
}

// PresignDownload 生成预签名下载地址。
func (d *S3Driver) PresignDownload(ctx context.Context, source *entity.StorageSource, virtualPath, disposition string, ttl time.Duration) (string, time.Time, error) {
	cfg, err := ParseS3ConfigJSON(source.ConfigJSON)
	if err != nil {
		return "", time.Time{}, err
	}
	key, err := buildS3ObjectKey(cfg, source.RootPath, virtualPath)
	if err != nil {
		return "", time.Time{}, err
	}
	client, presignClient, err := d.clientFactory.New(ctx, cfg)
	if err != nil {
		return "", time.Time{}, err
	}

	_, err = client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", time.Time{}, mapS3NotFound(err)
	}

	expiresAt := time.Now().Add(ttl)
	presigned, err := presignClient.PresignGetObject(
		ctx,
		&awss3.GetObjectInput{
			Bucket:                     aws.String(cfg.Bucket),
			Key:                        aws.String(key),
			ResponseContentDisposition: aws.String(buildContentDisposition(disposition, path.Base(virtualPath))),
		},
		func(options *awss3.PresignOptions) {
			options.Expires = ttl
		},
	)
	if err != nil {
		return "", time.Time{}, err
	}

	return presigned.URL, expiresAt, nil
}

// Delete 删除单个 S3 对象或伪目录。
func (d *S3Driver) Delete(ctx context.Context, source *entity.StorageSource, virtualPath string) error {
	cfg, err := ParseS3ConfigJSON(source.ConfigJSON)
	if err != nil {
		return err
	}
	client, _, err := d.clientFactory.New(ctx, cfg)
	if err != nil {
		return err
	}

	info, err := d.statPathWithClient(ctx, client, cfg, source.RootPath, virtualPath)
	if err != nil {
		return err
	}
	if info.isDir {
		return d.deleteDirectoryPrefix(ctx, client, cfg.Bucket, info.prefix)
	}
	return d.deleteObject(ctx, client, cfg.Bucket, info.key)
}

// InitMultipartUpload 初始化 multipart 上传并返回分片说明。
func (d *S3Driver) InitMultipartUpload(ctx context.Context, source *entity.StorageSource, req domainstorage.MultipartUploadRequest) (*domainstorage.MultipartUploadPlan, error) {
	cfg, err := ParseS3ConfigJSON(source.ConfigJSON)
	if err != nil {
		return nil, err
	}
	finalVirtualPath := joinVirtualPath(req.VirtualPath, req.Filename)
	key, err := buildS3ObjectKey(cfg, source.RootPath, finalVirtualPath)
	if err != nil {
		return nil, err
	}
	client, presignClient, err := d.clientFactory.New(ctx, cfg)
	if err != nil {
		return nil, err
	}

	input := &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(key),
	}
	if req.ContentType != "" {
		input.ContentType = aws.String(req.ContentType)
	}
	created, err := client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return nil, err
	}

	instructions := make([]domainstorage.MultipartUploadPartInstruction, 0, req.TotalParts)
	for index := range req.TotalParts {
		partNumber := int32(index + 1)
		presigned, presignErr := presignClient.PresignUploadPart(
			ctx,
			&awss3.UploadPartInput{
				Bucket:     aws.String(cfg.Bucket),
				Key:        aws.String(key),
				UploadId:   created.UploadId,
				PartNumber: aws.Int32(partNumber),
			},
			func(options *awss3.PresignOptions) {
				options.Expires = req.ExpiresIn
			},
		)
		if presignErr != nil {
			return nil, presignErr
		}

		start := int64(index) * req.PartSize
		end := start + req.PartSize - 1
		if end >= req.FileSize {
			end = req.FileSize - 1
		}
		instructions = append(instructions, domainstorage.MultipartUploadPartInstruction{
			Index:     index,
			Method:    "PUT",
			URL:       presigned.URL,
			Headers:   map[string]string{},
			ByteStart: start,
			ByteEnd:   end,
			ExpiresAt: time.Now().Add(req.ExpiresIn),
		})
	}

	return &domainstorage.MultipartUploadPlan{
		State: domainstorage.MultipartUploadState{
			RemoteUploadID: aws.ToString(created.UploadId),
			ObjectKey:      key,
			VirtualPath:    finalVirtualPath,
		},
		PartInstructions: instructions,
	}, nil
}

// CompleteMultipartUpload 完成 multipart 上传。
func (d *S3Driver) CompleteMultipartUpload(ctx context.Context, source *entity.StorageSource, state domainstorage.MultipartUploadState, parts []domainstorage.CompletedUploadPart) (*domainstorage.StorageEntry, error) {
	cfg, err := ParseS3ConfigJSON(source.ConfigJSON)
	if err != nil {
		return nil, err
	}
	client, _, err := d.clientFactory.New(ctx, cfg)
	if err != nil {
		return nil, err
	}

	sort.Slice(parts, func(i, j int) bool {
		return parts[i].Index < parts[j].Index
	})
	completedParts := make([]awss3types.CompletedPart, 0, len(parts))
	for _, part := range parts {
		partNumber := int32(part.Index + 1)
		completedParts = append(completedParts, awss3types.CompletedPart{
			ETag:       aws.String(part.ETag),
			PartNumber: aws.Int32(partNumber),
		})
	}

	_, err = client.CompleteMultipartUpload(ctx, &awss3.CompleteMultipartUploadInput{
		Bucket:   aws.String(cfg.Bucket),
		Key:      aws.String(state.ObjectKey),
		UploadId: aws.String(state.RemoteUploadID),
		MultipartUpload: &awss3types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return nil, err
	}

	head, err := client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(state.ObjectKey),
	})
	if err != nil {
		return nil, err
	}

	modifiedAt := aws.ToTime(head.LastModified)
	if modifiedAt.IsZero() {
		modifiedAt = time.Now()
	}
	return &domainstorage.StorageEntry{
		Name:       path.Base(state.VirtualPath),
		Path:       state.VirtualPath,
		IsDir:      false,
		Size:       aws.ToInt64(head.ContentLength),
		ETag:       strings.Trim(aws.ToString(head.ETag), `"`),
		ModifiedAt: modifiedAt,
	}, nil
}

type s3PathInfo struct {
	entry  domainstorage.StorageEntry
	isDir  bool
	key    string
	prefix string
}

func (d *S3Driver) renameOrCopy(ctx context.Context, source *entity.StorageSource, sourcePath, targetPath string, removeSource bool) (*domainstorage.StorageEntry, error) {
	cfg, err := ParseS3ConfigJSON(source.ConfigJSON)
	if err != nil {
		return nil, err
	}
	client, _, err := d.clientFactory.New(ctx, cfg)
	if err != nil {
		return nil, err
	}

	sourceInfo, err := d.statPathWithClient(ctx, client, cfg, source.RootPath, sourcePath)
	if err != nil {
		return nil, err
	}
	targetInfo, err := d.tryStatPathWithClient(ctx, client, cfg, source.RootPath, targetPath)
	if err != nil {
		return nil, err
	}
	if targetInfo != nil {
		return nil, fs.ErrExist
	}

	if sourceInfo.isDir {
		if err := d.copyDirectoryPrefix(ctx, client, cfg.Bucket, sourceInfo.prefix, prefixFromJoinedPath(cfg, path.Join(source.RootPath, targetPath))); err != nil {
			return nil, err
		}
		if removeSource {
			if err := d.deleteDirectoryPrefix(ctx, client, cfg.Bucket, sourceInfo.prefix); err != nil {
				return nil, err
			}
		}
		return &domainstorage.StorageEntry{
			Name:       path.Base(targetPath),
			Path:       targetPath,
			IsDir:      true,
			ModifiedAt: time.Now(),
		}, nil
	}

	targetKey, err := buildS3ObjectKey(cfg, source.RootPath, targetPath)
	if err != nil {
		return nil, err
	}
	if err := d.copyObject(ctx, client, cfg.Bucket, sourceInfo.key, targetKey); err != nil {
		return nil, err
	}
	if removeSource {
		if err := d.deleteObject(ctx, client, cfg.Bucket, sourceInfo.key); err != nil {
			return nil, err
		}
	}

	head, err := client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(targetKey),
	})
	if err != nil {
		return nil, err
	}
	modifiedAt := aws.ToTime(head.LastModified)
	if modifiedAt.IsZero() {
		modifiedAt = time.Now()
	}
	return &domainstorage.StorageEntry{
		Name:       path.Base(targetPath),
		Path:       targetPath,
		IsDir:      false,
		Size:       aws.ToInt64(head.ContentLength),
		ETag:       strings.Trim(aws.ToString(head.ETag), `"`),
		ModifiedAt: modifiedAt,
	}, nil
}

func (d *S3Driver) tryStatPathWithClient(ctx context.Context, client *awss3.Client, cfg S3Config, rootPath string, virtualPath string) (*s3PathInfo, error) {
	info, err := d.statPathWithClient(ctx, client, cfg, rootPath, virtualPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (d *S3Driver) statPath(ctx context.Context, source *entity.StorageSource, virtualPath string) (*s3PathInfo, error) {
	cfg, err := ParseS3ConfigJSON(source.ConfigJSON)
	if err != nil {
		return nil, err
	}
	client, _, err := d.clientFactory.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return d.statPathWithClient(ctx, client, cfg, source.RootPath, virtualPath)
}

func (d *S3Driver) statPathWithClient(ctx context.Context, client *awss3.Client, cfg S3Config, rootPath string, virtualPath string) (*s3PathInfo, error) {
	if virtualPath == "/" {
		return &s3PathInfo{
			entry: domainstorage.StorageEntry{
				Name:       "/",
				Path:       "/",
				IsDir:      true,
				ModifiedAt: time.Now(),
			},
			isDir:  true,
			prefix: prefixFromJoinedPath(cfg, rootPath),
		}, nil
	}

	key, err := buildS3ObjectKey(cfg, rootPath, virtualPath)
	if err != nil {
		return nil, err
	}
	head, err := client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		modifiedAt := aws.ToTime(head.LastModified)
		if modifiedAt.IsZero() {
			modifiedAt = time.Now()
		}
		return &s3PathInfo{
			entry: domainstorage.StorageEntry{
				Name:       path.Base(virtualPath),
				Path:       virtualPath,
				IsDir:      false,
				Size:       aws.ToInt64(head.ContentLength),
				ETag:       strings.Trim(aws.ToString(head.ETag), `"`),
				ModifiedAt: modifiedAt,
			},
			key:   key,
			isDir: false,
		}, nil
	}
	if !errors.Is(mapS3NotFound(err), os.ErrNotExist) {
		return nil, err
	}

	prefix, err := buildS3ListPrefix(cfg, rootPath, virtualPath)
	if err != nil {
		return nil, err
	}
	listed, err := client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{
		Bucket:  aws.String(cfg.Bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return nil, err
	}
	if len(listed.Contents) == 0 && len(listed.CommonPrefixes) == 0 {
		return nil, os.ErrNotExist
	}
	return &s3PathInfo{
		entry: domainstorage.StorageEntry{
			Name:       path.Base(virtualPath),
			Path:       virtualPath,
			IsDir:      true,
			ModifiedAt: time.Now(),
		},
		isDir:  true,
		prefix: prefix,
	}, nil
}

func (d *S3Driver) isDirectory(ctx context.Context, source *entity.StorageSource, virtualPath string) (bool, error) {
	info, err := d.statPath(ctx, source, virtualPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.isDir, nil
}

func (d *S3Driver) isDirectoryWithClient(ctx context.Context, client *awss3.Client, cfg S3Config, rootPath string, virtualPath string) (bool, error) {
	info, err := d.statPathWithClient(ctx, client, cfg, rootPath, virtualPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.isDir, nil
}

func (d *S3Driver) copyObject(ctx context.Context, client *awss3.Client, bucket string, sourceKey string, targetKey string) error {
	copySource := url.PathEscape(bucket + "/" + sourceKey)
	_, err := client.CopyObject(ctx, &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(targetKey),
		CopySource: aws.String(copySource),
	})
	return err
}

func (d *S3Driver) deleteObject(ctx context.Context, client *awss3.Client, bucket string, key string) error {
	_, err := client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return err
}

func (d *S3Driver) copyDirectoryPrefix(ctx context.Context, client *awss3.Client, bucket string, sourcePrefix string, targetPrefix string) error {
	paginator := awss3.NewListObjectsV2Paginator(client, &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(sourcePrefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, object := range page.Contents {
			sourceKey := aws.ToString(object.Key)
			if sourceKey == "" {
				continue
			}
			targetKey := targetPrefix + strings.TrimPrefix(sourceKey, sourcePrefix)
			if err := d.copyObject(ctx, client, bucket, sourceKey, targetKey); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *S3Driver) deleteDirectoryPrefix(ctx context.Context, client *awss3.Client, bucket string, prefix string) error {
	paginator := awss3.NewListObjectsV2Paginator(client, &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, object := range page.Contents {
			key := aws.ToString(object.Key)
			if key == "" {
				continue
			}
			if err := d.deleteObject(ctx, client, bucket, key); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildS3ListPrefix(cfg S3Config, rootPath string, virtualPath string) (string, error) {
	virtual, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return "", err
	}
	root, err := normalizeVirtualPath(rootPath)
	if err != nil {
		return "", err
	}
	joined := path.Join(root, virtual)
	return prefixFromJoinedPath(cfg, joined), nil
}

func buildS3SearchPrefix(cfg S3Config, rootPath string, pathPrefix string) (string, error) {
	if pathPrefix == "" {
		pathPrefix = "/"
	}
	return buildS3ListPrefix(cfg, rootPath, pathPrefix)
}

func buildS3ObjectKey(cfg S3Config, rootPath string, virtualPath string) (string, error) {
	virtual, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return "", err
	}
	if virtual == "/" {
		return "", fmt.Errorf("root path is not a file")
	}
	root, err := normalizeVirtualPath(rootPath)
	if err != nil {
		return "", err
	}
	joined := path.Join(root, virtual)
	return keyFromJoinedPath(cfg, joined), nil
}

func keyToVirtualPath(cfg S3Config, rootPath string, key string) (string, error) {
	root, err := normalizeVirtualPath(rootPath)
	if err != nil {
		return "", err
	}
	rootPrefix := prefixFromJoinedPath(cfg, root)
	trimmed := strings.TrimPrefix(key, rootPrefix)
	if trimmed == key {
		return "", fmt.Errorf("key %q out of root", key)
	}
	return normalizeVirtualPath("/" + strings.TrimPrefix(trimmed, "/"))
}

func prefixFromJoinedPath(cfg S3Config, joined string) string {
	key := keyFromJoinedPath(cfg, joined)
	if key == "" {
		return ""
	}
	return strings.TrimSuffix(key, "/") + "/"
}

func keyFromJoinedPath(cfg S3Config, joined string) string {
	parts := make([]string, 0, 2)
	if cfg.BasePrefix != "" {
		parts = append(parts, cfg.BasePrefix)
	}
	trimmed := strings.Trim(strings.TrimSpace(joined), "/")
	if trimmed != "" {
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, "/")
}

func normalizeVirtualPath(input string) (string, error) {
	if input == "" {
		input = "/"
	}
	if !strings.HasPrefix(input, "/") {
		return "", fmt.Errorf("path must start with slash")
	}
	cleaned := path.Clean(input)
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("path contains parent traversal")
	}
	return cleaned, nil
}

func joinVirtualPath(parent string, name string) string {
	if parent == "/" {
		return "/" + name
	}
	return path.Join(parent, name)
}

func nameFromChildPrefix(parentPrefix string, childPrefix string) string {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(childPrefix, parentPrefix), "/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return ""
	}
	return trimmed
}

func buildContentDisposition(disposition string, filename string) string {
	if disposition == "" {
		disposition = "attachment"
	}
	escaped := url.PathEscape(filename)
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disposition, filename, escaped)
}

func mapS3NotFound(err error) error {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "404":
			return os.ErrNotExist
		}
	}
	return err
}
