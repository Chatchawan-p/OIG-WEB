package evidence

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/oig-police/oig-web/internal/apperror"
)

var tinyPNG = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52}

func TestLocalStorageValidatesAndStoresImages(t *testing.T) {
	storage, err := NewLocal(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := storage.Save(context.Background(), "../../evidence.png", bytes.NewReader(tinyPNG))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if stored.ContentType != "image/png" || stored.OriginalName != "evidence.png" {
		t.Fatalf("unexpected stored file: %#v", stored)
	}
	reader, err := storage.Open(context.Background(), stored.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, _ := io.ReadAll(reader)
	if !bytes.Equal(data, tinyPNG) {
		t.Fatal("stored data differs")
	}
}

func TestLocalStorageRejectsTypeAndSize(t *testing.T) {
	storage, err := NewLocal(t.TempDir(), 16)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
	}{
		{name: "type", data: []byte("plain text")},
		{name: "size", data: append(tinyPNG, make([]byte, 20)...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := storage.Save(context.Background(), "file", bytes.NewReader(tt.data))
			var validation *apperror.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("Save() error = %v, want validation error", err)
			}
		})
	}
}
