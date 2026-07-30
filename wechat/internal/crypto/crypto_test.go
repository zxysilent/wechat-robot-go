package crypto_test

import (
	"bytes"
	"crypto/aes"
	crand "crypto/rand"
	"io"
	mrand "math/rand"
	"testing"
	"testing/iotest"

	"github.com/SpellingDragon/wechat-robot-go/wechat/internal/crypto"
)

func TestGenerateAESKey(t *testing.T) {
	key, err := crypto.GenerateAESKey()
	if err != nil {
		t.Fatalf("GenerateAESKey() error = %v", err)
	}
	if len(key) != 16 {
		t.Errorf("GenerateAESKey() key length = %d, want 16", len(key))
	}

	// Test that two keys are different (randomness)
	key2, err := crypto.GenerateAESKey()
	if err != nil {
		t.Fatalf("GenerateAESKey() second call error = %v", err)
	}
	if bytes.Equal(key, key2) {
		t.Error("GenerateAESKey() generated identical keys, expected different")
	}
}

func TestPKCS7Pad(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		blockSize int
		wantLen   int
	}{
		{
			name:      "empty data",
			data:      []byte{},
			blockSize: 16,
			wantLen:   16,
		},
		{
			name:      "data smaller than block",
			data:      []byte("hello"),
			blockSize: 16,
			wantLen:   16,
		},
		{
			name:      "data equals block size",
			data:      make([]byte, 16),
			blockSize: 16,
			wantLen:   32, // needs full block of padding
		},
		{
			name:      "data larger than block",
			data:      make([]byte, 20),
			blockSize: 16,
			wantLen:   32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crypto.PKCS7Pad(tt.data, tt.blockSize)
			if len(got) != tt.wantLen {
				t.Errorf("PKCS7Pad() length = %d, want %d", len(got), tt.wantLen)
			}
			if len(got)%tt.blockSize != 0 {
				t.Errorf("PKCS7Pad() length not multiple of block size")
			}
		})
	}
}

func TestPKCS7Unpad(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    []byte
		wantErr bool
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "valid padding 1 byte",
			data:    append([]byte("hello world!!!!"), byte(1)),
			want:    []byte("hello world!!!!"),
			wantErr: false,
		},
		{
			name:    "valid padding 5 bytes",
			data:    append([]byte("hello world"), bytes.Repeat([]byte{5}, 5)...),
			want:    []byte("hello world"),
			wantErr: false,
		},
		{
			name:    "invalid padding - zero",
			data:    append([]byte("hello"), byte(0)),
			wantErr: true,
		},
		{
			name:    "invalid padding - too large",
			data:    append([]byte("hi"), byte(20)),
			wantErr: true,
		},
		{
			name:    "invalid padding - inconsistent bytes",
			data:    append([]byte("hello world"), []byte{5, 5, 5, 4, 5}...),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := crypto.PKCS7Unpad(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("PKCS7Unpad() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !bytes.Equal(got, tt.want) {
				t.Errorf("PKCS7Unpad() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPKCS7PadUnpad_RoundTrip(t *testing.T) {
	testData := [][]byte{
		[]byte(""),
		[]byte("a"),
		[]byte("hello"),
		[]byte("exactly16bytes!!"),
		[]byte("this is a longer test string that spans multiple blocks"),
	}

	for _, data := range testData {
		padded := crypto.PKCS7Pad(data, aes.BlockSize)
		unpadded, err := crypto.PKCS7Unpad(padded)
		if err != nil {
			t.Errorf("Round trip failed for %q: %v", data, err)
			continue
		}
		if !bytes.Equal(unpadded, data) {
			t.Errorf("Round trip mismatch: got %q, want %q", unpadded, data)
		}
	}
}

func TestEncryptAESECB(t *testing.T) {
	key, _ := crypto.GenerateAESKey()

	tests := []struct {
		name      string
		plaintext []byte
		key       []byte
		wantErr   bool
	}{
		{
			name:      "valid encryption",
			plaintext: []byte("hello world"),
			key:       key,
			wantErr:   false,
		},
		{
			name:      "empty plaintext",
			plaintext: []byte{},
			key:       key,
			wantErr:   false,
		},
		{
			name:      "invalid key length",
			plaintext: []byte("hello"),
			key:       []byte("short"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := crypto.EncryptAESECB(tt.plaintext, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("EncryptAESECB() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got)%aes.BlockSize != 0 {
					t.Errorf("EncryptAESECB() output not multiple of block size")
				}
			}
		})
	}
}

func TestDecryptAESECB(t *testing.T) {
	key, _ := crypto.GenerateAESKey()

	// Create valid ciphertext first
	plaintext := []byte("test message")
	validCiphertext, _ := crypto.EncryptAESECB(plaintext, key)

	tests := []struct {
		name       string
		ciphertext []byte
		key        []byte
		wantErr    bool
	}{
		{
			name:       "valid decryption",
			ciphertext: validCiphertext,
			key:        key,
			wantErr:    false,
		},
		{
			name:       "invalid key length",
			ciphertext: validCiphertext,
			key:        []byte("short"),
			wantErr:    true,
		},
		{
			name:       "invalid ciphertext length",
			ciphertext: []byte("not multiple of 16"),
			key:        key,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := crypto.DecryptAESECB(tt.ciphertext, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecryptAESECB() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEncryptDecryptAESECB_RoundTrip(t *testing.T) {
	key, err := crypto.GenerateAESKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	testCases := [][]byte{
		[]byte(""),
		[]byte("hello"),
		[]byte("exactly16bytes!!"),
		[]byte("this is a much longer message that spans multiple AES blocks"),
		bytes.Repeat([]byte("a"), 1000),
	}

	for _, plaintext := range testCases {
		ciphertext, err := crypto.EncryptAESECB(plaintext, key)
		if err != nil {
			t.Errorf("Encrypt failed for len=%d: %v", len(plaintext), err)
			continue
		}

		decrypted, err := crypto.DecryptAESECB(ciphertext, key)
		if err != nil {
			t.Errorf("Decrypt failed for len=%d: %v", len(plaintext), err)
			continue
		}

		if !bytes.Equal(decrypted, plaintext) {
			t.Errorf("Round trip mismatch for len=%d", len(plaintext))
		}
	}
}

func TestDecryptAESECB_WrongKey(t *testing.T) {
	key1, _ := crypto.GenerateAESKey()
	key2, _ := crypto.GenerateAESKey()

	plaintext := []byte("secret message")
	ciphertext, err := crypto.EncryptAESECB(plaintext, key1)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Decrypting with wrong key should either error or produce wrong result
	decrypted, err := crypto.DecryptAESECB(ciphertext, key2)
	if err == nil && bytes.Equal(decrypted, plaintext) {
		t.Error("Decryption with wrong key should not produce original plaintext")
	}
}

// --- NewECBDecryptReader tests ---

func decryptViaReader(t *testing.T, src io.Reader, key []byte) ([]byte, error) {
	t.Helper()
	return io.ReadAll(crypto.NewECBDecryptReader(src, key))
}

func TestNewECBDecryptReader_EmptyCiphertext(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, 16)
	_, err := decryptViaReader(t, bytes.NewReader(nil), key)
	if err == nil {
		t.Fatal("NewECBDecryptReader with empty ciphertext: expected error, got nil")
	}
}

func TestNewECBDecryptReader_InvalidKeyLength(t *testing.T) {
	ciphertext := bytes.Repeat([]byte{0x22}, 16)
	for _, keyLen := range []int{0, 15, 17, 32} {
		_, err := decryptViaReader(t, bytes.NewReader(ciphertext), bytes.Repeat([]byte{0x01}, keyLen))
		if err == nil {
			t.Errorf("key length %d: expected error, got nil", keyLen)
		}
	}
}

func TestNewECBDecryptReader_NonBlockAlignedCiphertext(t *testing.T) {
	key := bytes.Repeat([]byte{0x33}, 16)
	for _, n := range []int{1, 15, 17, 31, 33} {
		_, err := decryptViaReader(t, bytes.NewReader(bytes.Repeat([]byte{0x44}, n)), key)
		if err == nil {
			t.Errorf("ciphertext length %d: expected error, got nil", n)
		}
	}
}

func TestNewECBDecryptReader_InvalidPadding(t *testing.T) {
	key := bytes.Repeat([]byte{0x55}, 16)
	plaintext := []byte("some plaintext data")
	ciphertext, err := crypto.EncryptAESECB(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptAESECB() error = %v", err)
	}
	// Corrupt the last ciphertext block so padding is invalid after decryption.
	corrupted := append([]byte(nil), ciphertext...)
	corrupted[len(corrupted)-1] ^= 0xFF
	if _, err := decryptViaReader(t, bytes.NewReader(corrupted), key); err == nil {
		t.Fatal("expected invalid padding error, got nil")
	}
}

func TestNewECBDecryptReader_MatchesDecryptAESECB(t *testing.T) {
	key, err := crypto.GenerateAESKey()
	if err != nil {
		t.Fatalf("GenerateAESKey() error = %v", err)
	}

	// Cover empty, single block, block boundary +/- 1, chunk boundary and
	// assorted random lengths.
	lengths := []int{0, 1, 15, 16, 17, 31, 32, 33, 255, 4096, 32*1024 - 1, 32 * 1024, 32*1024 + 1, 100000}
	for i := 0; i < 20; i++ {
		lengths = append(lengths, mrand.Intn(200000))
	}

	for _, n := range lengths {
		plaintext := make([]byte, n)
		if _, err := crand.Read(plaintext); err != nil {
			t.Fatalf("rand.Read() error = %v", err)
		}
		ciphertext, err := crypto.EncryptAESECB(plaintext, key)
		if err != nil {
			t.Fatalf("len=%d: EncryptAESECB() error = %v", n, err)
		}

		want, err := crypto.DecryptAESECB(ciphertext, key)
		if err != nil {
			t.Fatalf("len=%d: DecryptAESECB() error = %v", n, err)
		}
		got, err := decryptViaReader(t, bytes.NewReader(ciphertext), key)
		if err != nil {
			t.Fatalf("len=%d: NewECBDecryptReader error = %v", n, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("len=%d: streaming output differs from DecryptAESECB", n)
		}
	}
}

func TestNewECBDecryptReader_OneByteAtATimeSource(t *testing.T) {
	key := bytes.Repeat([]byte{0x66}, 16)
	plaintext := make([]byte, 1000)
	if _, err := crand.Read(plaintext); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	ciphertext, err := crypto.EncryptAESECB(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptAESECB() error = %v", err)
	}

	got, err := decryptViaReader(t, iotest.OneByteReader(bytes.NewReader(ciphertext)), key)
	if err != nil {
		t.Fatalf("NewECBDecryptReader with one-byte source error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatal("one-byte source: streaming output differs from original plaintext")
	}
}

func TestNewECBDecryptReader_SingleBlock(t *testing.T) {
	key := bytes.Repeat([]byte{0x77}, 16)
	plaintext := []byte("short") // pads into exactly one block
	ciphertext, err := crypto.EncryptAESECB(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptAESECB() error = %v", err)
	}
	if len(ciphertext) != aes.BlockSize {
		t.Fatalf("expected single-block ciphertext, got %d bytes", len(ciphertext))
	}
	got, err := decryptViaReader(t, bytes.NewReader(ciphertext), key)
	if err != nil {
		t.Fatalf("NewECBDecryptReader error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("got %q, want %q", got, plaintext)
	}
}
