package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// upload posts one file the way a browser form does.
func upload(e *env, c *http.Client, key, filename string, content []byte) (int, httpapi.Attachment, httpapi.Problem) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write(content)
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, e.url+"/api/v1/tickets/"+key+"/attachments", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", origin)
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var a httpapi.Attachment
	var p httpapi.Problem
	_ = json.Unmarshal(raw, &a)
	_ = json.Unmarshal(raw, &p)
	return res.StatusCode, a, p
}

func download(e *env, c *http.Client, id int64) (int, http.Header, []byte) {
	e.t.Helper()
	res, err := c.Get(fmt.Sprintf("%s/api/v1/attachments/%d", e.url, id))
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, raw
}

func TestUploadDownloadAndDeleteAttachments(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	pdf := []byte("%PDF-1.7 overtime report")
	code, a, _ := upload(e, w.pm, tk.Key, "Laporan Lembur.pdf", pdf)
	if code != http.StatusCreated || a.Filename != "Laporan Lembur.pdf" || a.ContentType != "application/pdf" ||
		a.SizeBytes != int64(len(pdf)) || a.Uploader.Name != "pm@example.com" {
		t.Fatalf("upload: %d %+v", code, a)
	}
	code, h, got := download(e, w.pm, a.Id)
	if code != http.StatusOK || !bytes.Equal(got, pdf) || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment") ||
		h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Content-Type") != "application/pdf" {
		t.Fatalf("download: %d %v", code, h)
	}
	_, png, _ := upload(e, w.pm, tk.Key, "screen.png", []byte("\x89PNG not really"))
	if _, h, _ := download(e, w.pm, png.Id); !strings.HasPrefix(h.Get("Content-Disposition"), "inline") {
		t.Fatalf("images display inline: %v", h)
	}
	var detail httpapi.Ticket
	if e.call(w.pm, http.MethodGet, "/tickets/"+tk.Key, nil, &detail); len(detail.Attachments) != 2 {
		t.Fatalf("ticket lists its files: %+v", detail.Attachments)
	}
	if code := e.call(w.pm, http.MethodDelete, fmt.Sprintf("/attachments/%d", a.Id), nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _, _ := download(e, w.pm, a.Id); code != http.StatusNotFound {
		t.Fatalf("a removed file: %d", code)
	}
	events, err := e.q.ListTicketEvents(context.Background(), tk.ID)
	if err != nil || len(events) != 3 || events[0].Action != "attachment_add" || events[2].Action != "attachment_delete" {
		t.Fatalf("history: %+v %v", events, err)
	}
}

// AC-TK-7, against the 64 KiB limit newEnv sets.
func TestAttachmentLimits(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	if code, _, p := upload(e, w.pm, tk.Key, "big.zip", bytes.Repeat([]byte("x"), 65<<10)); code != http.StatusRequestEntityTooLarge || p.Code != "file_too_large" {
		t.Errorf("too large: %d %+v", code, p)
	}
	for _, name := range []string{"run.exe", "page.svg", "index.html"} {
		if code, _, p := upload(e, w.pm, tk.Key, name, []byte("<x/>")); code != http.StatusUnsupportedMediaType || p.Code != "file_type_not_allowed" {
			t.Errorf("%s: %d %+v", name, code, p)
		}
	}
}

func TestAttachmentAccessFollowsTheTicket(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	owner, ou := e.signedIn("owner@example.com", false)
	e.seedMember(ou, w.p, "admin")
	ani, au := e.signedIn("ani@example.com", false)
	e.seedMember(au, w.p, "member")
	vera, vu := e.signedIn("vera@example.com", false)
	e.seedMember(vu, w.p, "viewer")
	forB := e.seedTicket(w.p, ou, "Client B export", &w.b)
	_, b, _ := upload(e, owner, forB.Key, "b.csv", []byte("a,b"))
	if code, _, _ := download(e, w.pm, b.Id); code != http.StatusNotFound {
		t.Errorf("a file of a hidden ticket: %d", code)
	}
	core := e.seedTicket(w.p, ou, "Shared thing", nil)
	_, f, _ := upload(e, ani, core.Key, "notes.txt", []byte("hello"))
	if code, _, _ := upload(e, vera, core.Key, "v.txt", []byte("x")); code != http.StatusForbidden {
		t.Errorf("a viewer uploads: %d", code)
	}
	path := fmt.Sprintf("/attachments/%d", f.Id)
	for _, c := range []*http.Client{vera, w.pm} {
		if code := e.call(c, http.MethodDelete, path, nil, nil); code != http.StatusForbidden {
			t.Errorf("not the uploader: %d", code)
		}
	}
	if code := e.call(owner, http.MethodDelete, path, nil, nil); code != http.StatusNoContent {
		t.Errorf("a project admin removes it: %d", code)
	}
}
