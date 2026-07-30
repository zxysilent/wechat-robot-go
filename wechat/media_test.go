package wechat

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/SpellingDragon/wechat-robot-go/wechat/internal/crypto"
	"github.com/SpellingDragon/wechat-robot-go/wechat/internal/media"
	"github.com/SpellingDragon/wechat-robot-go/wechat/internal/model"
)

// Tests for MediaManager through the public API (types.go)

func TestMediaManager_UploadFile_PublicAPI(t *testing.T) {
	testData := []byte("hello world test data")

	// Mock server for getuploadurl
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

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ilink/bot/getuploadurl" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ret": 0, "upload_param": "test-param"}`))
		}
	}))
	defer apiServer.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewClient(apiServer.URL, &http.Client{}, logger, "1.0.3")
	manager := media.NewMediaManager(client, client.HTTPClient(), logger)
	// Set CDN base URL separately
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

func TestMediaManager_UploadFileRetry_PublicAPI(t *testing.T) {
	testData := []byte("retry test data")
	attemptCount := 0

	cdnServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount == 1 {
			// First attempt returns 500
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
			return
		}
		// Second attempt succeeds
		w.Header().Set("x-encrypted-param", "success-param")
		w.WriteHeader(http.StatusOK)
	}))
	defer cdnServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ret": 0, "upload_param": "test-param"}`))
	}))
	defer apiServer.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewClient(apiServer.URL, &http.Client{}, logger, "1.0.3")
	manager := media.NewMediaManager(client, client.HTTPClient(), logger)
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

func TestBuildImageItem_PublicAPI(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewClient("http://unused", &http.Client{}, logger, "1.0.3")
	manager := media.NewMediaManager(client, client.HTTPClient(), logger)

	result := &media.UploadResult{
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
	if item.ImageItem.Media == nil {
		t.Fatal("ImageItem.Media is nil")
	}
	if item.ImageItem.Media.EncryptQueryParam != result.EncryptedParam {
		t.Errorf("EncryptQueryParam = %s, want %s", item.ImageItem.Media.EncryptQueryParam, result.EncryptedParam)
	}
	if item.ImageItem.Media.AESKey == "" {
		t.Error("ImageItem.Media.AESKey is empty")
	}
	// Verify AES key encoding: should be base64(hex_string)
	expectedAESKey := "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	if item.ImageItem.Media.AESKey != expectedAESKey {
		t.Errorf("AESKey = %s, want %s (base64 of hex string)", item.ImageItem.Media.AESKey, expectedAESKey)
	}
	if item.ImageItem.MidSize != result.CipherSize {
		t.Errorf("MidSize = %d, want %d", item.ImageItem.MidSize, result.CipherSize)
	}
}

func TestBuildFileItem_PublicAPI(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewClient("http://unused", &http.Client{}, logger, "1.0.3")
	manager := media.NewMediaManager(client, client.HTTPClient(), logger)

	result := &media.UploadResult{
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
	if item.FileItem.Media == nil {
		t.Fatal("FileItem.Media is nil")
	}
	if item.FileItem.FileName != "document.pdf" {
		t.Errorf("FileName = %s, want document.pdf", item.FileItem.FileName)
	}
	if item.FileItem.Length != "98765" {
		t.Errorf("Length = %s, want 98765", item.FileItem.Length)
	}
}

// TestStreamingDownloadAPIContract locks the exported streaming download API
// at compile time: the aliases, the sentinel error and the exact signatures of
// the four Bot streaming download methods.
func TestStreamingDownloadAPIContract(t *testing.T) {
	// DownloadOptions is an alias of the internal media type with a MaxSize field.
	var opts DownloadOptions = media.DownloadOptions{MaxSize: 1}
	if opts.MaxSize != 1 {
		t.Error("DownloadOptions.MaxSize not settable")
	}

	// ErrMaxSizeExceeded is the internal sentinel, detectable via errors.Is.
	if !errors.Is(ErrMaxSizeExceeded, media.ErrMaxSizeExceeded) {
		t.Error("ErrMaxSizeExceeded does not match media.ErrMaxSizeExceeded")
	}

	// Compile-time signature checks for the Bot streaming methods.
	var (
		_ func(ctx context.Context, cdnBaseURL string, img *ImageItem, w io.Writer, opts DownloadOptions) (int64, error) = (*Bot)(nil).DownloadImageFromItemTo
		_ func(ctx context.Context, voice *VoiceItem, cdnBaseURL string, w io.Writer, opts DownloadOptions) (int64, error) = (*Bot)(nil).DownloadVoiceTo
		_ func(ctx context.Context, file *FileItem, cdnBaseURL string, w io.Writer, opts DownloadOptions) (int64, error) = (*Bot)(nil).DownloadFileFromItemTo
		_ func(ctx context.Context, video *VideoItem, cdnBaseURL string, w io.Writer, opts DownloadOptions) (int64, error) = (*Bot)(nil).DownloadVideoFromItemTo
	)

	// DeclaredSize accessors must be callable through the alias types.
	var (
		_ func() int64 = (&FileItem{}).DeclaredSize
		_ func() int64 = (&VoiceItem{}).DeclaredSize
		_ func() int64 = (&VideoItem{}).DeclaredSize
		_ func() int64 = (&ImageItem{}).DeclaredSize
	)
	if got := (&FileItem{Length: "42"}).DeclaredSize(); got != 42 {
		t.Errorf("(*FileItem).DeclaredSize() via alias = %d, want 42", got)
	}
}
