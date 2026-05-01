package objectstorage

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/FangcunMount/component-base/pkg/logger"
	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	qrcodeasset "github.com/FangcunMount/qs-server/internal/apiserver/port/qrcodeasset"
)

const (
	DefaultQRCodeStorageDir = "/data/image/qrcode"
	DefaultQRCodeURLPrefix  = "https://qs.fangcunmount.cn/api/v1/qrcodes"
)

type QRCodeAssetStoreOptions struct {
	ObjectStore     objectstorageport.PublicObjectStore
	ObjectKeyPrefix string
	PublicURLPrefix string
	LocalStorageDir string
}

type qrCodeAssetStore struct {
	objectStore     objectstorageport.PublicObjectStore
	objectKeyPrefix string
	publicURLPrefix string
	localStorageDir string
}

var _ qrcodeasset.ImageStore = (*qrCodeAssetStore)(nil)

func NewQRCodeAssetStore(opts QRCodeAssetStoreOptions) qrcodeasset.ImageStore {
	publicURLPrefix := strings.TrimRight(strings.TrimSpace(opts.PublicURLPrefix), "/")
	if publicURLPrefix == "" {
		publicURLPrefix = DefaultQRCodeURLPrefix
	}
	localStorageDir := strings.TrimSpace(opts.LocalStorageDir)
	if localStorageDir == "" {
		localStorageDir = DefaultQRCodeStorageDir
	}
	return &qrCodeAssetStore{
		objectStore:     opts.ObjectStore,
		objectKeyPrefix: strings.Trim(opts.ObjectKeyPrefix, "/"),
		publicURLPrefix: publicURLPrefix,
		localStorageDir: localStorageDir,
	}
}

func (s *qrCodeAssetStore) StorePNG(ctx context.Context, fileName string, data []byte) (string, error) {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "", fmt.Errorf("qrcode file name is required")
	}
	if s.objectStore != nil {
		objectKey := s.buildObjectKey(fileName)
		if err := s.objectStore.Put(ctx, objectKey, "image/png", data); err != nil {
			return "", err
		}
		publicURL := s.buildPublicURL(fileName)
		logger.L(ctx).Infow("二维码对象上传成功",
			"action", "put_qrcode_object",
			"object_key", objectKey,
			"size", len(data),
			"qrcode_url", publicURL,
		)
		return publicURL, nil
	}

	if err := os.MkdirAll(s.localStorageDir, 0750); err != nil {
		return "", fmt.Errorf("创建二维码存储目录失败: %w", err)
	}
	filePath := filepath.Join(s.localStorageDir, fileName)
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return "", fmt.Errorf("写入二维码文件失败: %w", err)
	}
	logger.L(ctx).Infow("二维码文件保存成功",
		"action", "save_qrcode_file",
		"file_path", filePath,
		"size", len(data),
	)
	return s.buildPublicURL(fileName), nil
}

func (s *qrCodeAssetStore) buildObjectKey(fileName string) string {
	if s.objectKeyPrefix == "" {
		return fileName
	}
	return path.Join(s.objectKeyPrefix, fileName)
}

func (s *qrCodeAssetStore) buildPublicURL(fileName string) string {
	return fmt.Sprintf("%s/%s", s.publicURLPrefix, fileName)
}
