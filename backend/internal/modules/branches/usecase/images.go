package usecase

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
)

const (
	// MaxBranchImages caps the gallery of one branch.
	MaxBranchImages = 12
	// MaxBranchImageBytes caps one image upload (5 MiB).
	MaxBranchImageBytes = 5 << 20
)

var (
	ErrImageNotFound      = errors.New("branch image not found")
	ErrStorageUnavailable = errors.New("storage is not configured")
)

var imageMIMEToExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

// ImageRepository is the persistence port of branch galleries.
type ImageRepository interface {
	ImagesByBranches(ctx context.Context, orgID int64, branchIDs []int64) (map[int64][]db.BranchImage, error)
	CountImages(ctx context.Context, orgID, branchID int64) (int64, error)
	CreateImage(ctx context.Context, orgID, branchID int64, f repository.ImageFields) (db.BranchImage, error)
	GetImage(ctx context.Context, orgID, branchID int64, id uuid.UUID) (db.BranchImage, error)
	DeleteImage(ctx context.Context, orgID, branchID int64, id uuid.UUID) error
	SetImageSort(ctx context.Context, orgID, branchID int64, id uuid.UUID, order int32) error
	TouchBranch(ctx context.Context, orgID, branchID int64) error
	PublicImage(ctx context.Context, orgUUID, id uuid.UUID) (db.GetPublicBranchImageRow, error)
}

// SetMedia attaches the gallery store. Without it branches have no images.
func (s *Service) SetMedia(images ImageRepository, store storage.Driver) {
	s.images = images
	s.storage = store
}

// PublicImagePath is the API-owned public URL of a branch image; booking
// pages and the mobile app use it as is.
func PublicImagePath(orgUUID, imageUUID uuid.UUID) string {
	return "/v1/public/media/branches/" + orgUUID.String() + "/images/" + imageUUID.String()
}

func mapImage(row db.BranchImage, orgUUID uuid.UUID) model.BranchImage {
	return model.BranchImage{
		UUID: row.Uuid, PublicURL: PublicImagePath(orgUUID, row.Uuid), Filename: row.Filename,
		ContentType: row.ContentType, SizeBytes: row.SizeBytes, SortOrder: row.SortOrder,
		CreatedAt: row.CreatedAt.Time.UTC(),
	}
}

func scopeUUID(ctx context.Context) (uuid.UUID, bool) {
	scope, ok := orgctx.From(ctx)
	if !ok || scope.UUID == uuid.Nil {
		return uuid.Nil, false
	}
	return scope.UUID, true
}

// attachCovers sets cover_url on a page of branches (ids in the same order).
func (s *Service) attachCovers(ctx context.Context, orgID int64, ids []int64, out []model.Branch) error {
	orgUUID, ok := scopeUUID(ctx)
	if s.images == nil || !ok {
		return nil
	}
	images, err := s.images.ImagesByBranches(ctx, orgID, ids)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if list := images[id]; len(list) > 0 {
			url := PublicImagePath(orgUUID, list[0].Uuid)
			out[i].CoverURL = &url
		}
	}
	return nil
}

// attachImages fills the gallery and cover of one branch.
func (s *Service) attachImages(ctx context.Context, orgID, branchID int64, b *model.Branch) error {
	b.Images = []model.BranchImage{}
	orgUUID, ok := scopeUUID(ctx)
	if s.images == nil || !ok {
		return nil
	}
	images, err := s.images.ImagesByBranches(ctx, orgID, []int64{branchID})
	if err != nil {
		return err
	}
	for _, row := range images[branchID] {
		b.Images = append(b.Images, mapImage(row, orgUUID))
	}
	if len(b.Images) > 0 {
		url := b.Images[0].PublicURL
		b.CoverURL = &url
	}
	return nil
}

// ImageUpload is one uploaded file.
type ImageUpload struct {
	Body     io.Reader
	Size     int64
	Filename string
}

// UploadImage appends a photo to the branch gallery.
func (s *Service) UploadImage(ctx context.Context, orgID int64, branchUUID uuid.UUID, in ImageUpload) (model.BranchImage, error) {
	orgUUID, ok := scopeUUID(ctx)
	if s.images == nil || s.storage == nil || !ok {
		return model.BranchImage{}, ErrStorageUnavailable
	}
	branch, err := s.getBranch(ctx, orgID, branchUUID)
	if err != nil {
		return model.BranchImage{}, err
	}
	ve := &ValidationError{}
	count, err := s.images.CountImages(ctx, orgID, branch.ID)
	if err != nil {
		return model.BranchImage{}, err
	}
	switch {
	case count >= MaxBranchImages:
		ve.add("file", "too_many", "a branch can have at most 12 images")
	case in.Size <= 0:
		ve.add("file", "required", "file is required")
	case in.Size > MaxBranchImageBytes:
		ve.add("file", "too_large", "image must be at most 5 MiB")
	}
	if err := ve.orNil(); err != nil {
		return model.BranchImage{}, err
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
		return model.BranchImage{}, ve
	}
	imageUUID := uuid.New()
	key := storage.BranchImageObjectKey(orgUUID.String(), branch.Uuid.String(), imageUUID.String(), ext)
	filename := strings.TrimSpace(in.Filename)
	for utf8.RuneCountInString(filename) > 255 {
		r := []rune(filename)
		filename = string(r[len(r)-255:])
	}
	if err := s.storage.Upload(ctx, storage.File{
		Body: io.MultiReader(bytes.NewReader(head), in.Body), Size: in.Size, ContentType: mime, Filename: filename,
	}, key); err != nil {
		return model.BranchImage{}, err
	}
	row, err := s.images.CreateImage(ctx, orgID, branch.ID, repository.ImageFields{
		UUID: imageUUID, ObjectKey: key, Filename: filename, ContentType: mime, SizeBytes: in.Size,
	})
	if err != nil {
		_ = s.storage.Delete(ctx, key)
		return model.BranchImage{}, err
	}
	_ = s.images.TouchBranch(ctx, orgID, branch.ID)
	return mapImage(row, orgUUID), nil
}

// DeleteImage removes a gallery photo (row and object).
func (s *Service) DeleteImage(ctx context.Context, orgID int64, branchUUID, imageUUID uuid.UUID) error {
	if s.images == nil {
		return ErrImageNotFound
	}
	branch, err := s.getBranch(ctx, orgID, branchUUID)
	if err != nil {
		return err
	}
	img, err := s.images.GetImage(ctx, orgID, branch.ID, imageUUID)
	if err != nil {
		return mapNotFound(err, ErrImageNotFound)
	}
	if err := s.images.DeleteImage(ctx, orgID, branch.ID, imageUUID); err != nil {
		return mapNotFound(err, ErrImageNotFound)
	}
	if s.storage != nil {
		_ = s.storage.Delete(ctx, img.ObjectKey)
	}
	_ = s.images.TouchBranch(ctx, orgID, branch.ID)
	return nil
}

// ReorderImages sets the gallery order; the list names every image once.
// The first image is the cover.
func (s *Service) ReorderImages(ctx context.Context, orgID int64, branchUUID uuid.UUID, order []uuid.UUID) ([]model.BranchImage, error) {
	if s.images == nil {
		return nil, ErrImageNotFound
	}
	branch, err := s.getBranch(ctx, orgID, branchUUID)
	if err != nil {
		return nil, err
	}
	images, err := s.images.ImagesByBranches(ctx, orgID, []int64{branch.ID})
	if err != nil {
		return nil, err
	}
	existing := map[uuid.UUID]bool{}
	for _, img := range images[branch.ID] {
		existing[img.Uuid] = true
	}
	seen := map[uuid.UUID]bool{}
	valid := len(order) == len(existing)
	for _, id := range order {
		if !existing[id] || seen[id] {
			valid = false
		}
		seen[id] = true
	}
	if !valid {
		ve := &ValidationError{}
		ve.add("image_uuids", "invalid", "image_uuids must list every image of the branch once")
		return nil, ve
	}
	for i, id := range order {
		if err := s.images.SetImageSort(ctx, orgID, branch.ID, id, int32(i)); err != nil {
			return nil, mapNotFound(err, ErrImageNotFound)
		}
	}
	_ = s.images.TouchBranch(ctx, orgID, branch.ID)
	b, err := s.Get(ctx, orgID, branchUUID)
	if err != nil {
		return nil, err
	}
	return b.Images, nil
}

// PublicImage opens a branch photo for the public media route.
func (s *Service) PublicImage(ctx context.Context, orgUUID, imageUUID uuid.UUID) (io.ReadCloser, string, int64, error) {
	if s.images == nil || s.storage == nil {
		return nil, "", 0, ErrStorageUnavailable
	}
	row, err := s.images.PublicImage(ctx, orgUUID, imageUUID)
	if err != nil {
		return nil, "", 0, mapNotFound(err, ErrImageNotFound)
	}
	// Defence in depth: serve keys under the named organization only.
	if !storage.OrganizationOwnsKey(row.ObjectKey, orgUUID.String()) {
		return nil, "", 0, ErrImageNotFound
	}
	rc, size, err := s.storage.Download(ctx, row.ObjectKey)
	if err != nil {
		return nil, "", 0, ErrImageNotFound
	}
	if size <= 0 {
		size = row.SizeBytes
	}
	return rc, row.ContentType, size, nil
}
