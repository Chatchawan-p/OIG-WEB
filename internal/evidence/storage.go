// Package evidence stores validated disciplinary evidence outside MongoDB.
package evidence

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/platform/id"
)

type File struct {
	ID           string
	Path         string
	ContentType  string
	OriginalName string
}

type Storage interface {
	Save(context.Context, string, io.Reader) (File, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}

type Local struct {
	root     string
	maxBytes int64
}

func NewLocal(root string, maxBytes int64) (*Local, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve evidence directory: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create evidence directory: %w", err)
	}
	return &Local{root: root, maxBytes: maxBytes}, nil
}

func (l *Local) Save(ctx context.Context, originalName string, reader io.Reader) (File, error) {
	if err := ctx.Err(); err != nil {
		return File{}, err
	}
	data, err := io.ReadAll(io.LimitReader(reader, l.maxBytes+1))
	if err != nil {
		return File{}, fmt.Errorf("read evidence: %w", err)
	}
	validation := &apperror.ValidationError{}
	if len(data) == 0 {
		validation.Add("evidence", "image is empty")
	}
	if int64(len(data)) > l.maxBytes {
		validation.Add("evidence", fmt.Sprintf("image exceeds %d bytes", l.maxBytes))
	}
	contentType := http.DetectContentType(data)
	extension := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
	}[contentType]
	if extension == "" {
		validation.Add("evidence", "only JPEG, PNG, and WebP images are allowed")
	}
	if err := validation.OrNil(); err != nil {
		return File{}, err
	}

	evidenceID, err := id.New("evd")
	if err != nil {
		return File{}, err
	}
	name := evidenceID + extension
	path := filepath.Join(l.root, name)
	if !strings.HasPrefix(path, l.root+string(os.PathSeparator)) {
		return File{}, errors.New("invalid evidence path")
	}
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return File{}, fmt.Errorf("create evidence file: %w", err)
	}
	writeErr := error(nil)
	if _, err := io.Copy(file, bytes.NewReader(data)); err != nil {
		writeErr = err
	}
	if err := file.Close(); writeErr == nil {
		writeErr = err
	}
	if writeErr != nil {
		_ = os.Remove(temporary)
		return File{}, fmt.Errorf("write evidence file: %w", writeErr)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return File{}, fmt.Errorf("finalize evidence file: %w", err)
	}
	return File{ID: evidenceID, Path: path, ContentType: contentType, OriginalName: filepath.Base(originalName)}, nil
}

func (l *Local) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	clean, err := filepath.Abs(path)
	if err != nil || !strings.HasPrefix(clean, l.root+string(os.PathSeparator)) {
		return nil, apperror.ErrNotFound
	}
	file, err := os.Open(clean)
	if errors.Is(err, os.ErrNotExist) {
		return nil, apperror.ErrNotFound
	}
	return file, err
}

func (l *Local) Delete(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	clean, err := filepath.Abs(path)
	if err != nil || !strings.HasPrefix(clean, l.root+string(os.PathSeparator)) {
		return apperror.ErrNotFound
	}
	err = os.Remove(clean)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
