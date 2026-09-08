package userprofile

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/emergent-company/emergent.memory/pkg/auth"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

var (
	testPNGBytes  = append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0x00}, 64)...)
	testJPEGBytes = append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}, bytes.Repeat([]byte{0x00}, 64)...)
	testSVGBytes  = []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>`)
	testTextBytes = []byte("plain text, not an image")
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newAvatarTestHandler(t *testing.T, repo profileRepo, store avatarStore) *Handler {
	t.Helper()
	return NewHandler(newTestAvatarService(repo, store))
}

func newAvatarEchoContext(method string, body io.Reader, contentType string, userID string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, "/api/user/avatar", body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	if userID != "" {
		c.Set(string(auth.UserContextKey), &auth.AuthUser{ID: userID, Sub: "sub-1"})
	}
	return c, rec
}

func multipartAvatarBody(t *testing.T, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := new(bytes.Buffer)
	w := multipart.NewWriter(buf)
	part, err := w.CreateFormFile("file", "avatar")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("failed to write form data: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}
	return buf, w.FormDataContentType()
}

func assertHandlerOutcome(t *testing.T, err error, rec *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if err != nil {
		assertAppErrorStatus(t, err, wantStatus)
		return
	}
	if rec.Code != wantStatus {
		t.Errorf("status = %d, want %d (body: %s)", rec.Code, wantStatus, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Upload handler tests
// ---------------------------------------------------------------------------

func TestUpload_ValidPNG_ReturnsProfile(t *testing.T) {
	repo := newFakeProfileRepo()
	repo.seed(Profile{ID: "profile-1", ZitadelUserID: "zitadel-1"})
	store := newFakeAvatarStore(true)
	h := newAvatarTestHandler(t, repo, store)

	body, ct := multipartAvatarBody(t, testPNGBytes)
	c, rec := newAvatarEchoContext(http.MethodPut, body, ct, "profile-1")

	err := h.Upload(c)
	assertHandlerOutcome(t, err, rec, http.StatusOK)

	var dto ProfileDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if dto.AvatarObjectKey == nil || *dto.AvatarObjectKey == "" {
		t.Fatalf("expected avatar object key in response, got %v", dto.AvatarObjectKey)
	}
	if dto.AvatarURL == "" {
		t.Errorf("expected avatarUrl in response")
	}
}

func TestUpload_ValidJPEG_ReturnsProfile(t *testing.T) {
	repo := newFakeProfileRepo()
	repo.seed(Profile{ID: "profile-1", ZitadelUserID: "zitadel-1"})
	store := newFakeAvatarStore(true)
	h := newAvatarTestHandler(t, repo, store)

	body, ct := multipartAvatarBody(t, testJPEGBytes)
	c, rec := newAvatarEchoContext(http.MethodPut, body, ct, "profile-1")

	err := h.Upload(c)
	assertHandlerOutcome(t, err, rec, http.StatusOK)

	var dto ProfileDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if dto.AvatarObjectKey == nil {
		t.Errorf("expected avatar object key in response")
	}
}

func TestUpload_SVG_Rejected(t *testing.T) {
	repo := newFakeProfileRepo()
	repo.seed(Profile{ID: "profile-1", ZitadelUserID: "zitadel-1"})
	store := newFakeAvatarStore(true)
	h := newAvatarTestHandler(t, repo, store)

	body, ct := multipartAvatarBody(t, testSVGBytes)
	c, rec := newAvatarEchoContext(http.MethodPut, body, ct, "profile-1")

	err := h.Upload(c)
	assertHandlerOutcome(t, err, rec, http.StatusBadRequest)
	if len(store.objects) != 0 {
		t.Errorf("expected no upload for rejected image type")
	}
}

func TestUpload_NonImage_Rejected(t *testing.T) {
	repo := newFakeProfileRepo()
	repo.seed(Profile{ID: "profile-1", ZitadelUserID: "zitadel-1"})
	store := newFakeAvatarStore(true)
	h := newAvatarTestHandler(t, repo, store)

	body, ct := multipartAvatarBody(t, testTextBytes)
	c, rec := newAvatarEchoContext(http.MethodPut, body, ct, "profile-1")

	err := h.Upload(c)
	assertHandlerOutcome(t, err, rec, http.StatusBadRequest)
	if len(store.objects) != 0 {
		t.Errorf("expected no upload for rejected content")
	}
}

func TestUpload_Oversize_Rejected(t *testing.T) {
	repo := newFakeProfileRepo()
	repo.seed(Profile{ID: "profile-1", ZitadelUserID: "zitadel-1"})
	store := newFakeAvatarStore(true)
	h := newAvatarTestHandler(t, repo, store)

	// Pad well past maxAvatarSize so the whole multipart body exceeds the
	// MaxBytesReader cap, not just the file contents.
	oversize := append(testPNGBytes, bytes.Repeat([]byte{0x00}, maxAvatarSize+2048)...)
	body, ct := multipartAvatarBody(t, oversize)
	c, rec := newAvatarEchoContext(http.MethodPut, body, ct, "profile-1")

	err := h.Upload(c)
	assertHandlerOutcome(t, err, rec, http.StatusRequestEntityTooLarge)
	if len(store.objects) != 0 {
		t.Errorf("expected no upload for oversize file")
	}
}

func TestUpload_MissingFile_Rejected(t *testing.T) {
	repo := newFakeProfileRepo()
	repo.seed(Profile{ID: "profile-1", ZitadelUserID: "zitadel-1"})
	store := newFakeAvatarStore(true)
	h := newAvatarTestHandler(t, repo, store)

	buf := new(bytes.Buffer)
	w := multipart.NewWriter(buf)
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}
	c, rec := newAvatarEchoContext(http.MethodPut, buf, w.FormDataContentType(), "profile-1")

	err := h.Upload(c)
	assertHandlerOutcome(t, err, rec, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// GetAvatar handler tests
// ---------------------------------------------------------------------------

func TestGetAvatar_StreamsImage(t *testing.T) {
	key := "avatars/abc.png"
	repo := newFakeProfileRepo()
	repo.seed(Profile{ID: "profile-1", ZitadelUserID: "zitadel-1", AvatarObjectKey: &key})
	store := newFakeAvatarStore(true)
	store.objects[key] = testPNGBytes
	h := newAvatarTestHandler(t, repo, store)

	c, rec := newAvatarEchoContext(http.MethodGet, nil, "", "profile-1")

	err := h.GetAvatar(c)
	assertHandlerOutcome(t, err, rec, http.StatusOK)

	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want %q", ct, "image/png")
	}
	if !bytes.Equal(rec.Body.Bytes(), testPNGBytes) {
		t.Errorf("body = %d bytes, want %d bytes", rec.Body.Len(), len(testPNGBytes))
	}
}

func TestGetAvatar_NoAvatar_Returns404(t *testing.T) {
	repo := newFakeProfileRepo()
	repo.seed(Profile{ID: "profile-1", ZitadelUserID: "zitadel-1"})
	store := newFakeAvatarStore(true)
	h := newAvatarTestHandler(t, repo, store)

	c, rec := newAvatarEchoContext(http.MethodGet, nil, "", "profile-1")

	err := h.GetAvatar(c)
	assertHandlerOutcome(t, err, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// DeleteAvatar handler tests
// ---------------------------------------------------------------------------

func TestDeleteAvatar_ClearsAvatar(t *testing.T) {
	key := "avatars/abc.png"
	repo := newFakeProfileRepo()
	repo.seed(Profile{ID: "profile-1", ZitadelUserID: "zitadel-1", AvatarObjectKey: &key})
	store := newFakeAvatarStore(true)
	store.objects[key] = testPNGBytes
	h := newAvatarTestHandler(t, repo, store)

	c, rec := newAvatarEchoContext(http.MethodDelete, nil, "", "profile-1")

	err := h.DeleteAvatar(c)
	assertHandlerOutcome(t, err, rec, http.StatusOK)

	if _, ok := store.objects[key]; ok {
		t.Errorf("avatar object %q was not deleted", key)
	}
	var dto ProfileDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if dto.AvatarObjectKey != nil || dto.AvatarURL != "" {
		t.Errorf("expected cleared avatar in response, got key=%v url=%q", dto.AvatarObjectKey, dto.AvatarURL)
	}
	if strings.Contains(rec.Body.String(), "avatarUrl") {
		t.Errorf("expected avatarUrl omitted from response, body: %s", rec.Body.String())
	}
}
