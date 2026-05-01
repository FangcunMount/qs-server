package objectstorage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
)

type publicObjectStoreStub struct {
	key         string
	contentType string
	body        []byte
}

func (s *publicObjectStoreStub) Put(_ context.Context, key string, contentType string, body []byte) error {
	s.key = key
	s.contentType = contentType
	s.body = append([]byte(nil), body...)
	return nil
}

func (*publicObjectStoreStub) Get(context.Context, string) (*objectstorageport.ObjectReader, error) {
	return nil, nil
}

func TestQRCodeAssetStoreStoresPNGInObjectStore(t *testing.T) {
	t.Parallel()

	objectStore := &publicObjectStoreStub{}
	store := NewQRCodeAssetStore(QRCodeAssetStoreOptions{
		ObjectStore:     objectStore,
		ObjectKeyPrefix: "nested/qrcode",
		PublicURLPrefix: "https://cdn.example.com/qrcodes/",
	})

	got, err := store.StorePNG(context.Background(), "entry.png", []byte("png-data"))
	if err != nil {
		t.Fatalf("StorePNG() error = %v", err)
	}
	if got != "https://cdn.example.com/qrcodes/entry.png" {
		t.Fatalf("public URL = %q", got)
	}
	if objectStore.key != "nested/qrcode/entry.png" || objectStore.contentType != "image/png" || string(objectStore.body) != "png-data" {
		t.Fatalf("unexpected object write: key=%q contentType=%q body=%q", objectStore.key, objectStore.contentType, objectStore.body)
	}
}

func TestQRCodeAssetStoreFallsBackToLocalFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewQRCodeAssetStore(QRCodeAssetStoreOptions{
		PublicURLPrefix: "https://api.example.com/api/v1/qrcodes",
		LocalStorageDir: dir,
	})

	got, err := store.StorePNG(context.Background(), "local.png", []byte("local-png-data"))
	if err != nil {
		t.Fatalf("StorePNG() error = %v", err)
	}
	if got != "https://api.example.com/api/v1/qrcodes/local.png" {
		t.Fatalf("public URL = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, "local.png"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "local-png-data" {
		t.Fatalf("stored data = %q", data)
	}
}

var _ objectstorageport.PublicObjectStore = (*publicObjectStoreStub)(nil)
