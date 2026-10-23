package usecase

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
)

// MaxProductImageBytes caps one cover upload (5 MiB).
const MaxProductImageBytes = 5 << 20

var (
	ErrImageNotFound      = errors.New("product image not found")
	ErrStorageUnavailable = errors.New("storage is not configured")
)

var imageMIMEToExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

// ImageUpload is one uploaded file.
type ImageUpload struct {
	Body io.Reader
	Size int64
}

// UploadImage replaces the cover image of a product.
func (s *Service) UploadImage(ctx context.Context, orgID int64, id uuid.UUID, in ImageUpload) (model.Product, error) {
	orgUUID, ok := scopeUUID(ctx)
	if s.storage == nil || !ok {
		return model.Product{}, ErrStorageUnavailable
	}
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Product{}, err
	}
	ve := &ValidationError{}
	switch {
	case in.Size <= 0:
		ve.add("file", "required", "file is required")
	case in.Size > MaxProductImageBytes:
		ve.add("file", "too_large", "image must be at most 5 MiB")
	}
	if err := ve.orNil(); err != nil {
		return model.Product{}, err
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(in.Body, head)
	head = head[:n]
	// Sniff the bytes: a client-declared type must not smuggle HTML / SVG
	// into public storage.
	mime := strings.ToLower(http.DetectContentType(head))
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	ext, ok := imageMIMEToExt[mime]
	if !ok {
		ve.add("file", "invalid_type", "image must be JPEG, PNG or WebP")
		return model.Product{}, ve
	}
	imageUUID := uuid.New()
	key := storage.ProductImageObjectKey(orgUUID.String(), row.Uuid.String(), imageUUID.String(), ext)
	if err := s.storage.Upload(ctx, storage.File{
		Body: io.MultiReader(bytes.NewReader(head), in.Body), Size: in.Size, ContentType: mime,
	}, key); err != nil {
		return model.Product{}, err
	}
	prev, err := q.SetProductImage(ctx, db.SetProductImageParams{
		OrganizationID: orgID, ID: row.ID,
		ImageUuid:        pgtype.UUID{Bytes: imageUUID, Valid: true},
		ImageKey:         pgtype.Text{String: key, Valid: true},
		ImageContentType: pgtype.Text{String: mime, Valid: true},
		ImageSizeBytes:   pgtype.Int8{Int64: in.Size, Valid: true},
	})
	if err != nil {
		_ = s.storage.Delete(ctx, key)
		return model.Product{}, mapNotFound(err, ErrNotFound)
	}
	if prev.Valid && prev.String != "" {
		_ = s.storage.Delete(ctx, prev.String)
	}
	return s.Get(ctx, orgID, id)
}

// DeleteImage removes the cover image of a product.
func (s *Service) DeleteImage(ctx context.Context, orgID int64, id uuid.UUID) (model.Product, error) {
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Product{}, err
	}
	if !row.ImageUuid.Valid {
		return model.Product{}, ErrImageNotFound
	}
	prev, err := q.SetProductImage(ctx, db.SetProductImageParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return model.Product{}, mapNotFound(err, ErrNotFound)
	}
	if s.storage != nil && prev.Valid && prev.String != "" {
		_ = s.storage.Delete(ctx, prev.String)
	}
	return s.Get(ctx, orgID, id)
}

// PublicImage opens a product cover for the public media route.
func (s *Service) PublicImage(ctx context.Context, orgUUID, imageUUID uuid.UUID) (io.ReadCloser, string, int64, error) {
	if s.storage == nil {
		return nil, "", 0, ErrStorageUnavailable
	}
	row, err := s.repo.Q().GetProductPublicImage(ctx, db.GetProductPublicImageParams{
		OrganizationUuid: orgUUID, ImageUuid: pgtype.UUID{Bytes: imageUUID, Valid: true},
	})
	if err != nil || !row.ImageKey.Valid {
		return nil, "", 0, ErrImageNotFound
	}
	// Defence in depth: serve keys under the named organization only.
	if !storage.OrganizationOwnsKey(row.ImageKey.String, orgUUID.String()) {
		return nil, "", 0, ErrImageNotFound
	}
	rc, size, err := s.storage.Download(ctx, row.ImageKey.String)
	if err != nil {
		return nil, "", 0, ErrImageNotFound
	}
	if size <= 0 {
		size = row.ImageSizeBytes.Int64
	}
	return rc, row.ImageContentType.String, size, nil
}
