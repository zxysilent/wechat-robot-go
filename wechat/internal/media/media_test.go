package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/SpellingDragon/wechat-robot-go/wechat/internal/crypto"
	"github.com/SpellingDragon/wechat-robot-go/wechat/internal/model"
)

// mockAPIClient implements APIClient interface for testing.
type mockAPIClient struct {
	PostFunc func(ctx context.Context, path string, body, result interface{}) error
}

func (m *mockAPIClient) Post(ctx context.Context, path string, body, result interface{}) error {
	if m.PostFunc != nil {
		return m.PostFunc(ctx, path, body, result)
	}
	return nil
}

func TestMediaManager_UploadFile(t *testing.T) {
	testData := []byte("hello world test data")

	// Mock server for CDN upload
	var capturedUploadData []byte
	cdnServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		capturedUploadData, _ = io.ReadAll(r.Body)
		w.Header().Set("x-encrypted-param", "test-encrypted-param")
		w.WriteHeader(http.StatusOK)
	}))
	defer cdnServer.Close()

	// Mock API client
	apiClient := &mockAPIClient{
		PostFunc: func(ctx context.Context, path string, body, result interface{}) error {
			if path == "/ilink/bot/getuploadurl" {
				if resp, ok := result.(*UploadURLResponse); ok {
					resp.Ret = 0
					resp.UploadParam = "test-param"
				}
			}
			return nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewMediaManager(apiClient, &http.Client{}, logger)
	manager.SetCDNBaseURL(cdnServer.URL)

	result, err := manager.UploadFile(context.Background(), testData, "test-user-id", "image")
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}

	// Verify result
	if result.AESKey == "" {
		t.Error("UploadFile() AESKey is empty")
	}
	if result.FileKey == "" {
		t.Error("UploadFile() FileKey is empty")
	}
	if result.EncryptedParam != "test-encrypted-param" {
		t.Errorf("UploadFile() EncryptedParam = %s, want test-encrypted-param", result.EncryptedParam)
	}
	if result.FileSize != len(testData) {
		t.Errorf("UploadFile() FileSize = %d, want %d", result.FileSize, len(testData))
	}

	// Verify uploaded data can be decrypted back
	aesKey, _ := hex.DecodeString(result.AESKey)
	decrypted, err := crypto.DecryptAESECB(capturedUploadData, aesKey)
	if err != nil {
		t.Fatalf("decrypt captured data error = %v", err)
	}
	if !bytes.Equal(decrypted, testData) {
		t.Errorf("decrypted data mismatch: got %s, want %s", decrypted, testData)
	}
}

func TestMediaManager_UploadFileRetry(t *testing.T) {
	testData := []byte("retry test data")
	attemptCount := 0

	cdnServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount == 1 {
			// First attempt returns 500
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte("internal error")); err != nil {
				t.Logf("Write error: %v", err)
			}
			return
		}
		// Second attempt succeeds
		w.Header().Set("x-encrypted-param", "success-param")
		if _, err := w.Write([]byte{}); err != nil {
			t.Logf("Write error: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer cdnServer.Close()

	apiClient := &mockAPIClient{
		PostFunc: func(ctx context.Context, path string, body, result interface{}) error {
			if resp, ok := result.(*UploadURLResponse); ok {
				resp.Ret = 0
				resp.UploadParam = "test-param"
			}
			return nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewMediaManager(apiClient, &http.Client{}, logger)
	manager.SetCDNBaseURL(cdnServer.URL)

	result, err := manager.UploadFile(context.Background(), testData, "test-user-id", "file")
	if err != nil {
		t.Fatalf("UploadFile() with retry error = %v", err)
	}

	if attemptCount != 2 {
		t.Errorf("expected 2 attempts, got %d", attemptCount)
	}
	if result.EncryptedParam != "success-param" {
		t.Errorf("EncryptedParam = %s, want success-param", result.EncryptedParam)
	}
}

func TestMediaManager_UploadFile4xxNoRetry(t *testing.T) {
	testData := []byte("4xx test data")
	attemptCount := 0

	cdnServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte("bad request")); err != nil {
			t.Logf("Write error: %v", err)
		}
	}))
	defer cdnServer.Close()

	apiClient := &mockAPIClient{
		PostFunc: func(ctx context.Context, path string, body, result interface{}) error {
			if resp, ok := result.(*UploadURLResponse); ok {
				resp.Ret = 0
				resp.UploadParam = "test-param"
			}
			return nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewMediaManager(apiClient, &http.Client{}, logger)
	manager.SetCDNBaseURL(cdnServer.URL)

	_, err := manager.UploadFile(context.Background(), testData, "test-user-id", "file")
	if err == nil {
		t.Fatal("UploadFile() expected error for 4xx")
	}

	if attemptCount != 1 {
		t.Errorf("expected 1 attempt (no retry for 4xx), got %d", attemptCount)
	}
}

func TestMediaManager_DownloadFile(t *testing.T) {
	originalData := []byte("original test content for download")
	aesKey := bytes.Repeat([]byte{0x42}, 16)
	aesKeyHex := hex.EncodeToString(aesKey)
	// DownloadFileWithKey expects base64-encoded key
	aesKeyBase64 := base64.StdEncoding.EncodeToString([]byte(aesKeyHex))

	// Encrypt the data
	encrypted, err := crypto.EncryptAESECB(originalData, aesKey)
	if err != nil {
		t.Fatalf("encrypt test data error = %v", err)
	}

	// Mock CDN server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(encrypted); err != nil {
			t.Logf("Write error: %v", err)
		}
	}))
	defer server.Close()

	apiClient := &mockAPIClient{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewMediaManager(apiClient, &http.Client{}, logger)

	result, err := manager.DownloadFile(context.Background(), server.URL, aesKeyBase64)
	if err != nil {
		t.Fatalf("DownloadFile() error = %v", err)
	}

	if !bytes.Equal(result, originalData) {
		t.Errorf("DownloadFile() result mismatch:\ngot:  %s\nwant: %s", result, originalData)
	}
}

func TestMediaManager_DownloadFileInvalidKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(bytes.Repeat([]byte{0x01}, 16))
	}))
	defer server.Close()

	apiClient := &mockAPIClient{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewMediaManager(apiClient, &http.Client{}, logger)

	// Invalid hex string
	_, err := manager.DownloadFile(context.Background(), server.URL, "invalid-hex")
	if err == nil {
		t.Error("DownloadFile() expected error for invalid hex key")
	}
}

func TestBuildImageItem(t *testing.T) {
	apiClient := &mockAPIClient{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewMediaManager(apiClient, &http.Client{}, logger)

	result := &UploadResult{
		AESKey:         "0123456789abcdef0123456789abcdef",
		FileKey:        "fedcba9876543210fedcba9876543210",
		EncryptedParam: "test-encrypted-param",
		FileSize:       12345,
		CipherSize:     12368, // encrypted size (padded to 16 bytes)
	}

	item := manager.BuildImageItem(result, 800, 600)

	if item.Type != model.ItemTypeImage {
		t.Errorf("Type = %d, want %d", item.Type, model.ItemTypeImage)
	}
	if item.ImageItem == nil {
		t.Fatal("ImageItem is nil")
	}
	// ImageItem now uses Media field with CDNMedia struct
	if item.ImageItem.Media == nil {
		t.Fatal("ImageItem.Media is nil")
	}
	if item.ImageItem.Media.EncryptQueryParam != result.EncryptedParam {
		t.Errorf("EncryptQueryParam = %s, want %s", item.ImageItem.Media.EncryptQueryParam, result.EncryptedParam)
	}
	if item.ImageItem.Media.AESKey == "" {
		t.Error("ImageItem.Media.AESKey is empty")
	}
	// Verify AES key encoding: should be base64(hex_string), NOT base64(raw_bytes)
	// hex_string = "0123456789abcdef0123456789abcdef" (32 chars)
	// base64(hex_string) = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" (44 chars)
	expectedAESKey := "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	if item.ImageItem.Media.AESKey != expectedAESKey {
		t.Errorf("AESKey = %s, want %s (base64 of hex string)", item.ImageItem.Media.AESKey, expectedAESKey)
	}
	if item.ImageItem.Media.EncryptType != 1 {
		t.Errorf("EncryptType = %d, want 1", item.ImageItem.Media.EncryptType)
	}
	if item.ImageItem.MidSize != result.CipherSize {
		t.Errorf("MidSize = %d, want %d", item.ImageItem.MidSize, result.CipherSize)
	}
}

func TestBuildFileItem(t *testing.T) {
	apiClient := &mockAPIClient{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewMediaManager(apiClient, &http.Client{}, logger)

	result := &UploadResult{
		AESKey:         "fedcba9876543210fedcba9876543210",
		FileKey:        "0123456789abcdef0123456789abcdef",
		EncryptedParam: "file-encrypted-param",
		FileSize:       98765,
		CipherSize:     98776, // padded size
	}

	item := manager.BuildFileItem(result, "document.pdf")

	if item.Type != model.ItemTypeFile {
		t.Errorf("Type = %d, want %d", item.Type, model.ItemTypeFile)
	}
	if item.FileItem == nil {
		t.Fatal("FileItem is nil")
	}
	// FileItem now uses Media field
	if item.FileItem.Media == nil {
		t.Fatal("FileItem.Media is nil")
	}
	if item.FileItem.Media.EncryptQueryParam != result.EncryptedParam {
		t.Errorf("EncryptQueryParam = %s, want %s", item.FileItem.Media.EncryptQueryParam, result.EncryptedParam)
	}
	if item.FileItem.Media.AESKey == "" {
		t.Error("FileItem.Media.AESKey is empty")
	}
	if item.FileItem.FileName != "document.pdf" {
		t.Errorf("FileName = %s, want document.pdf", item.FileItem.FileName)
	}
	if item.FileItem.Length != "98765" {
		t.Errorf("Length = %s, want 98765", item.FileItem.Length)
	}
}

func TestBuildVideoItem(t *testing.T) {
	apiClient := &mockAPIClient{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewMediaManager(apiClient, &http.Client{}, logger)

	result := &UploadResult{
		AESKey:         "abcdef0123456789abcdef0123456789",
		FileKey:        "fedcba9876543210fedcba9876543210",
		EncryptedParam: "video-encrypted-param",
		FileSize:       1234567,
		CipherSize:     1234576, // padded size
	}

	item := manager.BuildVideoItem(result, 1920, 1080, 30000)

	if item.Type != model.ItemTypeVideo {
		t.Errorf("Type = %d, want %d", item.Type, model.ItemTypeVideo)
	}
	if item.VideoItem == nil {
		t.Fatal("VideoItem is nil")
	}
	// VideoItem now uses Media field
	if item.VideoItem.Media == nil {
		t.Fatal("VideoItem.Media is nil")
	}
	if item.VideoItem.Media.EncryptQueryParam != result.EncryptedParam {
		t.Errorf("EncryptQueryParam = %s, want %s", item.VideoItem.Media.EncryptQueryParam, result.EncryptedParam)
	}
	if item.VideoItem.Media.AESKey == "" {
		t.Error("VideoItem.Media.AESKey is empty")
	}
	if item.VideoItem.VideoSize != result.FileSize {
		t.Errorf("VideoSize = %d, want %d", item.VideoItem.VideoSize, result.FileSize)
	}
	if item.VideoItem.PlayLength != 30000 {
		t.Errorf("PlayLength = %d, want 30000", item.VideoItem.PlayLength)
	}
	if item.VideoItem.ThumbWidth != 1920 {
		t.Errorf("ThumbWidth = %d, want 1920", item.VideoItem.ThumbWidth)
	}
	if item.VideoItem.ThumbHeight != 1080 {
		t.Errorf("ThumbHeight = %d, want 1080", item.VideoItem.ThumbHeight)
	}
}

func TestBuildVoiceItem(t *testing.T) {
	apiClient := &mockAPIClient{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewMediaManager(apiClient, &http.Client{}, logger)

	result := &UploadResult{
		AESKey:         "abcdef0123456789abcdef0123456789",
		FileKey:        "fedcba9876543210fedcba9876543210",
		EncryptedParam: "voice-encrypted-param",
		FileSize:       54321,
		CipherSize:     54336,
	}

	item := manager.BuildVoiceItem(result, 5000)

	if item.Type != model.ItemTypeVoice {
		t.Errorf("Type = %d, want %d", item.Type, model.ItemTypeVoice)
	}
	if item.VoiceItem == nil {
		t.Fatal("VoiceItem is nil")
	}
	if item.VoiceItem.Media == nil {
		t.Fatal("VoiceItem.Media is nil")
	}
	if item.VoiceItem.Media.EncryptQueryParam != result.EncryptedParam {
		t.Errorf("EncryptQueryParam = %s, want %s", item.VoiceItem.Media.EncryptQueryParam, result.EncryptedParam)
	}
	if item.VoiceItem.Duration != 5000 {
		t.Errorf("Duration = %d, want 5000", item.VoiceItem.Duration)
	}
}

func TestIsHexString(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"valid 32 hex chars", "0123456789abcdef0123456789abcdef", true},
		{"valid uppercase hex", "0123456789ABCDEF0123456789ABCDEF", true},
		{"invalid length", "0123456789abcdef", false},
		{"invalid chars", "0123456789ghijkl0123456789abcdef", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHexString(tt.s); got != tt.want {
				t.Errorf("isHexString(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

// --- Streaming download (DownloadToWriter) tests ---

// newTestManager returns a MediaManager wired to a mock API client.
func newTestManager() *MediaManager {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	return NewMediaManager(&mockAPIClient{}, &http.Client{}, logger)
}

// makeStreamFixture encrypts size bytes of deterministic plaintext and returns
// (plaintext, ciphertext, base64 key string).
func makeStreamFixture(t *testing.T, size int) ([]byte, []byte, string) {
	t.Helper()
	plaintext := make([]byte, size)
	for i := range plaintext {
		plaintext[i] = byte(i % 251)
	}
	aesKey := bytes.Repeat([]byte{0x42}, 16)
	encrypted, err := crypto.EncryptAESECB(plaintext, aesKey)
	if err != nil {
		t.Fatalf("encrypt fixture error = %v", err)
	}
	return plaintext, encrypted, base64.StdEncoding.EncodeToString(aesKey)
}

func TestDownloadToWriter_Basic(t *testing.T) {
	plaintext, encrypted, keyStr := makeStreamFixture(t, 100000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(encrypted)
	}))
	defer server.Close()

	manager := newTestManager()
	var buf bytes.Buffer
	n, err := manager.DownloadToWriter(context.Background(), server.URL, keyStr, &buf, DownloadOptions{})
	if err != nil {
		t.Fatalf("DownloadToWriter() error = %v", err)
	}
	if n != int64(len(plaintext)) {
		t.Errorf("DownloadToWriter() n = %d, want %d", n, len(plaintext))
	}
	if !bytes.Equal(buf.Bytes(), plaintext) {
		t.Error("DownloadToWriter() output differs from plaintext")
	}
}

func TestDownloadToWriter_HexEncodedKey(t *testing.T) {
	// Key given as base64(32 hex chars), same format as DownloadFileWithKey supports.
	plaintext := []byte("hex key format plaintext")
	aesKey := bytes.Repeat([]byte{0x24}, 16)
	encrypted, err := crypto.EncryptAESECB(plaintext, aesKey)
	if err != nil {
		t.Fatalf("encrypt fixture error = %v", err)
	}
	keyStr := base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(aesKey)))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(encrypted)
	}))
	defer server.Close()

	manager := newTestManager()
	var buf bytes.Buffer
	if _, err := manager.DownloadToWriter(context.Background(), server.URL, keyStr, &buf, DownloadOptions{}); err != nil {
		t.Fatalf("DownloadToWriter() error = %v", err)
	}
	if !bytes.Equal(buf.Bytes(), plaintext) {
		t.Error("DownloadToWriter() output differs from plaintext")
	}
}

func TestDownloadToWriter_MaxSizeBoundary(t *testing.T) {
	plaintext, encrypted, keyStr := makeStreamFixture(t, 1000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(encrypted)
	}))
	defer server.Close()

	manager := newTestManager()

	// MaxSize exactly equal to the ciphertext size must pass.
	var buf bytes.Buffer
	n, err := manager.DownloadToWriter(context.Background(), server.URL, keyStr, &buf,
		DownloadOptions{MaxSize: int64(len(encrypted))})
	if err != nil {
		t.Fatalf("MaxSize == ciphertext size: unexpected error = %v", err)
	}
	if n != int64(len(plaintext)) || !bytes.Equal(buf.Bytes(), plaintext) {
		t.Error("MaxSize == ciphertext size: output mismatch")
	}

	// One byte less must be rejected with ErrMaxSizeExceeded.
	buf.Reset()
	_, err = manager.DownloadToWriter(context.Background(), server.URL, keyStr, &buf,
		DownloadOptions{MaxSize: int64(len(encrypted)) - 1})
	if !errors.Is(err, ErrMaxSizeExceeded) {
		t.Fatalf("MaxSize == ciphertext size - 1: error = %v, want ErrMaxSizeExceeded", err)
	}
}

func TestDownloadToWriter_ContentLengthGate(t *testing.T) {
	// The server declares an oversized Content-Length; the client must reject
	// via the header check without decrypting any body bytes.
	_, encrypted, keyStr := makeStreamFixture(t, 100000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(encrypted)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(encrypted)
	}))
	defer server.Close()

	manager := newTestManager()
	var buf bytes.Buffer
	n, err := manager.DownloadToWriter(context.Background(), server.URL, keyStr, &buf,
		DownloadOptions{MaxSize: 16})
	if !errors.Is(err, ErrMaxSizeExceeded) {
		t.Fatalf("error = %v, want ErrMaxSizeExceeded", err)
	}
	if n != 0 {
		t.Errorf("n = %d, want 0 (body must not be decrypted)", n)
	}
	if buf.Len() != 0 {
		t.Errorf("writer received %d bytes, want 0 (Content-Length gate must fire before reading body)", buf.Len())
	}
}

func TestDownloadToWriter_ChunkedStreamTruncated(t *testing.T) {
	// No Content-Length (chunked transfer): the in-flight ciphertext counter
	// must abort the download mid-stream once MaxSize is exceeded, and the
	// writer must have received a partial (incomplete) output.
	plaintext, encrypted, keyStr := makeStreamFixture(t, 256*1024)
	maxSize := int64(100 * 1024)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}
		// Stream in small chunks and flush to force chunked encoding.
		for off := 0; off < len(encrypted); off += 8 * 1024 {
			end := off + 8*1024
			if end > len(encrypted) {
				end = len(encrypted)
			}
			if _, err := w.Write(encrypted[off:end]); err != nil {
				return // client aborted, expected
			}
			flusher.Flush()
		}
	}))
	defer server.Close()

	manager := newTestManager()
	var buf bytes.Buffer
	n, err := manager.DownloadToWriter(context.Background(), server.URL, keyStr, &buf,
		DownloadOptions{MaxSize: maxSize})
	if !errors.Is(err, ErrMaxSizeExceeded) {
		t.Fatalf("error = %v, want ErrMaxSizeExceeded", err)
	}
	// Contract: on error the writer may hold a partial, incomplete output.
	if n != int64(buf.Len()) {
		t.Errorf("returned n = %d, but writer holds %d bytes", n, buf.Len())
	}
	if buf.Len() >= len(plaintext) {
		t.Errorf("writer received %d bytes, want a truncated output (< %d)", buf.Len(), len(plaintext))
	}
	if buf.Len() > 0 && !bytes.Equal(buf.Bytes(), plaintext[:buf.Len()]) {
		t.Error("partial output is not a prefix of the plaintext")
	}
}

func TestDownloadToWriter_MisreportedContentLength(t *testing.T) {
	// The server hides the real size (identity framing, no Content-Length,
	// Connection: close), so the header gate cannot fire; the byte counter
	// must still enforce MaxSize.
	_, encrypted, keyStr := makeStreamFixture(t, 64*1024)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("response writer does not support hijacking")
			return
		}
		conn, bw, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack error: %v", err)
			return
		}
		defer conn.Close()
		_, _ = bw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nConnection: close\r\n\r\n")
		_, _ = bw.Write(encrypted)
		_ = bw.Flush()
	}))
	defer server.Close()

	manager := newTestManager()
	var buf bytes.Buffer
	_, err := manager.DownloadToWriter(context.Background(), server.URL, keyStr, &buf,
		DownloadOptions{MaxSize: 1024})
	if !errors.Is(err, ErrMaxSizeExceeded) {
		t.Fatalf("error = %v, want ErrMaxSizeExceeded", err)
	}
}

func TestDownloadToWriter_MatchesDownloadFileWithKey(t *testing.T) {
	// The streaming API must produce byte-identical output to the legacy
	// []byte API against the same fake CDN.
	for _, size := range []int{0, 1, 15, 16, 17, 1000, 32 * 1024, 100000} {
		_, encrypted, keyStr := makeStreamFixture(t, size)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(encrypted)
		}))

		manager := newTestManager()

		legacy, err := manager.DownloadFileWithKey(context.Background(), server.URL, keyStr)
		if err != nil {
			server.Close()
			t.Fatalf("size=%d: DownloadFileWithKey() error = %v", size, err)
		}

		var buf bytes.Buffer
		n, err := manager.DownloadToWriter(context.Background(), server.URL, keyStr, &buf, DownloadOptions{})
		server.Close()
		if err != nil {
			t.Fatalf("size=%d: DownloadToWriter() error = %v", size, err)
		}
		if n != int64(len(legacy)) || !bytes.Equal(buf.Bytes(), legacy) {
			t.Fatalf("size=%d: streaming output differs from legacy API", size)
		}
	}
}

func TestDownloadToWriter_HTTPErrorStatus(t *testing.T) {
	_, _, keyStr := makeStreamFixture(t, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	manager := newTestManager()
	var buf bytes.Buffer
	if _, err := manager.DownloadToWriter(context.Background(), server.URL, keyStr, &buf, DownloadOptions{}); err == nil {
		t.Fatal("expected error for http 404, got nil")
	}
}

func TestDownloadItemToVariants(t *testing.T) {
	plaintext, encrypted, keyStr := makeStreamFixture(t, 5000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(encrypted)
	}))
	defer server.Close()

	manager := newTestManager()
	cdnMedia := &model.CDNMedia{EncryptQueryParam: "test-param", AESKey: keyStr}

	tests := []struct {
		name string
		call func(w io.Writer) (int64, error)
	}{
		{"image", func(w io.Writer) (int64, error) {
			return manager.DownloadImageTo(context.Background(), server.URL, &model.ImageItem{Media: cdnMedia}, w, DownloadOptions{})
		}},
		{"voice", func(w io.Writer) (int64, error) {
			return manager.DownloadVoiceTo(context.Background(), server.URL, &model.VoiceItem{Media: cdnMedia}, w, DownloadOptions{})
		}},
		{"file", func(w io.Writer) (int64, error) {
			return manager.DownloadFileItemTo(context.Background(), server.URL, &model.FileItem{Media: cdnMedia}, w, DownloadOptions{})
		}},
		{"video", func(w io.Writer) (int64, error) {
			return manager.DownloadVideoItemTo(context.Background(), server.URL, &model.VideoItem{Media: cdnMedia}, w, DownloadOptions{})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			n, err := tt.call(&buf)
			if err != nil {
				t.Fatalf("Download%sTo error = %v", tt.name, err)
			}
			if n != int64(len(plaintext)) || !bytes.Equal(buf.Bytes(), plaintext) {
				t.Errorf("Download%sTo output mismatch", tt.name)
			}
		})
	}
}

func TestDownloadItemToVariants_NoMedia(t *testing.T) {
	manager := newTestManager()
	var buf bytes.Buffer
	ctx := context.Background()

	if _, err := manager.DownloadImageTo(ctx, "http://x", &model.ImageItem{}, &buf, DownloadOptions{}); err == nil {
		t.Error("DownloadImageTo: expected error for missing media")
	}
	if _, err := manager.DownloadVoiceTo(ctx, "http://x", &model.VoiceItem{}, &buf, DownloadOptions{}); err == nil {
		t.Error("DownloadVoiceTo: expected error for missing media")
	}
	if _, err := manager.DownloadFileItemTo(ctx, "http://x", &model.FileItem{}, &buf, DownloadOptions{}); err == nil {
		t.Error("DownloadFileItemTo: expected error for missing media")
	}
	if _, err := manager.DownloadVideoItemTo(ctx, "http://x", &model.VideoItem{}, &buf, DownloadOptions{}); err == nil {
		t.Error("DownloadVideoItemTo: expected error for missing media")
	}
}
