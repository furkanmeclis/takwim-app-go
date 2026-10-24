package usecase

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxDocumentBytes caps a receipt / invoice file (10 MiB).
const MaxDocumentBytes = 10 << 20

var documentTypes = map[string]string{
	"application/pdf": "pdf", "image/jpeg": "jpg", "image/png": "png", "image/webp": "webp",
}

// FileUpload is an uploaded document.
type FileUpload struct {
	Body     io.Reader
	Size     int64
	Filename string
}

// DocumentFile is an open document for streaming.
type DocumentFile struct {
	Body        io.ReadCloser
	ContentType string
	Name        string
	Size        int64
}

// sniffDocument returns the type of an accepted file from its first bytes
// (the client-declared type is ignored): PDF, JPEG, PNG or WebP.
func sniffDocument(head []byte) (string, string, bool) {
	mime := strings.ToLower(http.DetectContentType(head))
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	ext, ok := documentTypes[mime]
	return mime, ext, ok
}

func cleanFilename(name, ext string) string {
	name = strings.TrimSpace(path.Base(strings.ReplaceAll(name, "\\", "/")))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "belge." + ext
	}
	for utf8.RuneCountInString(name) > 255 {
		r := []rune(name)
		name = string(r[len(r)-255:])
	}
	return name
}

// documentEntry loads a record that can carry a document (not a reversal).
func (s *Service) documentEntry(ctx context.Context, orgID int64, id uuid.UUID) (db.GetFinanceEntryRow, error) {
	row, err := s.loadEntry(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return row, err
	}
	if row.EntryType != model.TypeEntry {
		return row, ErrNotReversible
	}
	return row, nil
}

// UploadDocument stores (or replaces) the document of a record in private
// storage under the organization prefix. A replaced document's object is
// kept: a correction may share it and the audit trail stays complete.
func (s *Service) UploadDocument(ctx context.Context, orgID int64, id uuid.UUID, in FileUpload) (model.Entry, model.Entry, error) {
	scope, ok := orgctx.From(ctx)
	if s.storage == nil || !ok || scope.UUID == uuid.Nil {
		return model.Entry{}, model.Entry{}, ErrStorageUnavailable
	}
	row, err := s.documentEntry(ctx, orgID, id)
	if err != nil {
		return model.Entry{}, model.Entry{}, err
	}
	switch {
	case in.Size <= 0 || in.Body == nil:
		return model.Entry{}, model.Entry{}, invalid("file", "required", "file is required")
	case in.Size > MaxDocumentBytes:
		return model.Entry{}, model.Entry{}, invalid("file", "too_large", "file must be at most 10 MiB")
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(in.Body, head)
	head = head[:n]
	mime, ext, ok := sniffDocument(head)
	if !ok {
		return model.Entry{}, model.Entry{}, invalid("file", "invalid_type", "file must be a PDF, JPEG, PNG or WebP")
	}
	key := storage.FinanceDocumentObjectKey(scope.UUID.String(), id.String(), uuid.NewString(), ext)
	name := cleanFilename(in.Filename, ext)
	if err := s.storage.Upload(ctx, storage.File{
		Body: io.MultiReader(bytes.NewReader(head), in.Body), Size: in.Size, ContentType: mime, Filename: name,
	}, key); err != nil {
		return model.Entry{}, model.Entry{}, err
	}
	if _, err := s.repo.Q().SetFinanceEntryDocument(ctx, db.SetFinanceEntryDocumentParams{
		OrganizationID: orgID, ID: row.ID, DocumentKey: pgtype.Text{String: key, Valid: true}, DocumentName: name,
		DocumentContentType: mime, DocumentSizeBytes: in.Size,
	}); err != nil {
		_ = s.storage.Delete(ctx, key)
		return model.Entry{}, model.Entry{}, err
	}
	after, err := s.GetEntry(ctx, orgID, id)
	return mapEntry(row), after, err
}

// OpenDocument opens the document of a record for streaming; only keys
// under the organization's prefix are served.
func (s *Service) OpenDocument(ctx context.Context, orgID int64, id uuid.UUID) (DocumentFile, error) {
	scope, ok := orgctx.From(ctx)
	if s.storage == nil || !ok {
		return DocumentFile{}, ErrStorageUnavailable
	}
	row, err := s.loadEntry(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return DocumentFile{}, err
	}
	if !row.DocumentKey.Valid || !storage.OrganizationOwnsKey(row.DocumentKey.String, scope.UUID.String()) {
		return DocumentFile{}, ErrNoDocument
	}
	rc, size, err := s.storage.Download(ctx, row.DocumentKey.String)
	if err != nil {
		return DocumentFile{}, ErrNoDocument
	}
	if size <= 0 {
		size = row.DocumentSizeBytes
	}
	return DocumentFile{Body: rc, ContentType: row.DocumentContentType, Name: row.DocumentName, Size: size}, nil
}

// RemoveDocument detaches the document of a record (the object is kept,
// see UploadDocument).
func (s *Service) RemoveDocument(ctx context.Context, orgID int64, id uuid.UUID) (model.Entry, model.Entry, error) {
	row, err := s.documentEntry(ctx, orgID, id)
	if err != nil {
		return model.Entry{}, model.Entry{}, err
	}
	if !row.DocumentKey.Valid {
		return model.Entry{}, model.Entry{}, ErrNoDocument
	}
	if _, err := s.repo.Q().SetFinanceEntryDocument(ctx, db.SetFinanceEntryDocumentParams{OrganizationID: orgID, ID: row.ID}); err != nil {
		return model.Entry{}, model.Entry{}, err
	}
	after, err := s.GetEntry(ctx, orgID, id)
	return mapEntry(row), after, err
}
