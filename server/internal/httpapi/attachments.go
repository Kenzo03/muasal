package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

var errTooLarge = errors.New("the file is larger than the limit")

// attachmentTypes are the files tickets accept (FSD §8.7), by extension. SVG
// and HTML stay out because they could run script when opened.
var attachmentTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
	".pdf":  "application/pdf",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ppt":  "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".odt":  "application/vnd.oasis.opendocument.text",
	".ods":  "application/vnd.oasis.opendocument.spreadsheet",
	".txt":  "text/plain; charset=utf-8",
	".log":  "text/plain; charset=utf-8",
	".csv":  "text/csv; charset=utf-8",
	".zip":  "application/zip",
}

// UploadAttachment stores the multipart field "file" under its SHA-256, so a
// file uploaded twice is kept once.
func (s *Server) UploadAttachment(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	limit := s.cfg.AttachmentMaxBytes
	r.Body = http.MaxBytesReader(w, r.Body, limit+1<<20) // the file plus the multipart framing
	part, err := filePart(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_upload", "Send the file as multipart/form-data in a field named file")
		return
	}
	defer part.Close()
	name := filepath.Base(part.FileName())
	ctype, allowed := attachmentTypes[strings.ToLower(filepath.Ext(name))]
	if !allowed {
		writeProblem(w, http.StatusUnsupportedMediaType, "file_type_not_allowed",
			"Attach images, PDF, Office documents, text, CSV, logs or ZIP files")
		return
	}
	sum, size, err := s.storeFile(part, limit)
	if errors.Is(err, errTooLarge) {
		writeProblem(w, http.StatusRequestEntityTooLarge, "file_too_large", fmt.Sprintf("File is larger than %d MB", limit>>20))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	var a db.Attachment
	err = s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if a, err = q.CreateAttachment(ctx, db.CreateAttachmentParams{
			TicketID: row.Ticket.ID, UploaderID: pc.user.ID, Filename: name, ContentType: ctype, SizeBytes: size, Sha256: sum,
		}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", row.Ticket.ID, "attachment_add",
			map[string]any{"attachment_id": a.ID, "filename": name})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAPIAttachment(a, pc.user.Name))
}

// DownloadAttachment serves a file after its ticket's visibility check (FSD
// §8.7): images inline, everything else as a download, never sniffed.
func (s *Server) DownloadAttachment(w http.ResponseWriter, r *http.Request, id int64) {
	_, a, ok := s.attachmentFor(w, r, id, access.Viewer)
	if !ok {
		return
	}
	f, err := os.Open(s.attachmentPath(a.Sha256))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()
	disposition := "attachment"
	if strings.HasPrefix(a.ContentType, "image/") {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": a.Filename}))
	w.Header().Set("Content-Length", strconv.FormatInt(a.SizeBytes, 10))
	_, _ = io.Copy(w, f)
}

// DeleteAttachment removes a file from its ticket; the uploader or a project
// admin may. The stored bytes stay, since another upload may share them.
func (s *Server) DeleteAttachment(w http.ResponseWriter, r *http.Request, id int64) {
	pc, a, ok := s.attachmentFor(w, r, id, access.Member)
	if !ok {
		return
	}
	if a.UploaderID != pc.user.ID && !pc.scope.Allows(access.Admin) {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only the uploader or a project admin can remove a file")
		return
	}
	ctx := r.Context()
	err := s.inTx(ctx, func(q *db.Queries) error {
		if err := q.DeleteAttachment(ctx, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", a.TicketID, "attachment_delete",
			map[string]any{"attachment_id": id, "filename": a.Filename})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// attachmentFor loads a live attachment through its ticket's checks.
func (s *Server) attachmentFor(w http.ResponseWriter, r *http.Request, id int64, need string) (projectCtx, db.Attachment, bool) {
	if s.requireUser(w, r) == nil {
		return projectCtx{}, db.Attachment{}, false
	}
	row, err := s.q.GetAttachment(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Attachment not found")
		return projectCtx{}, db.Attachment{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.Attachment{}, false
	}
	pc, _, ok := s.ticketFor(w, r, row.TicketKey, need)
	if !ok {
		return projectCtx{}, db.Attachment{}, false
	}
	return pc, row.Attachment, true
}

// filePart finds the multipart field named file.
func filePart(r *http.Request) (*multipart.Part, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	for {
		p, err := mr.NextPart()
		if err != nil {
			return nil, err // io.EOF: the form has no file field
		}
		if p.FormName() == "file" {
			return p, nil
		}
		_ = p.Close()
	}
}

// storeFile copies at most limit bytes into the attachments directory, named
// by their SHA-256. A longer file is errTooLarge and leaves nothing behind.
func (s *Server) storeFile(src io.Reader, limit int64) ([]byte, int64, error) {
	tmp, err := os.CreateTemp(s.cfg.AttachmentsDir, "upload-*")
	if err != nil {
		return nil, 0, err
	}
	defer os.Remove(tmp.Name()) // a no-op once the file is renamed into place
	defer tmp.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(src, limit+1))
	var tooBig *http.MaxBytesError
	if n > limit || errors.As(err, &tooBig) {
		return nil, 0, errTooLarge
	}
	if err != nil {
		return nil, 0, err
	}
	if err := tmp.Close(); err != nil {
		return nil, 0, err
	}
	sum := h.Sum(nil)
	path := s.attachmentPath(sum)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, 0, err
	}
	return sum, n, os.Rename(tmp.Name(), path)
}

// attachmentPath is <ATTACHMENTS_DIR>/<hex[:2]>/<hex> (FSD §8.7).
func (s *Server) attachmentPath(sum []byte) string {
	h := hex.EncodeToString(sum)
	return filepath.Join(s.cfg.AttachmentsDir, h[:2], h)
}
