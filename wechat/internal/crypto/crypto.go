package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// GenerateAESKey generates a random 16-byte AES-128 key.
func GenerateAESKey() ([]byte, error) {
	key := make([]byte, 16)
	_, err := rand.Read(key)
	return key, err
}

// PKCS7Pad pads data to a multiple of blockSize using PKCS7.
func PKCS7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padtext := make([]byte, padding)
	for i := range padtext {
		padtext[i] = byte(padding)
	}
	return append(data, padtext...)
}

// PKCS7Unpad removes PKCS7 padding.
func PKCS7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padding := int(data[len(data)-1])
	if padding > len(data) || padding > aes.BlockSize || padding == 0 {
		return nil, fmt.Errorf("invalid padding size: %d", padding)
	}
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding byte")
		}
	}
	return data[:len(data)-padding], nil
}

// EncryptAESECB encrypts data using AES-128-ECB mode with PKCS7 padding.
func EncryptAESECB(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	padded := PKCS7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(padded))

	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(ciphertext[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}

	return ciphertext, nil
}

// DecryptAESECB decrypts data using AES-128-ECB mode and removes PKCS7 padding.
func DecryptAESECB(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length %d is not a multiple of block size %d", len(ciphertext), aes.BlockSize)
	}

	plaintext := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += aes.BlockSize {
		block.Decrypt(plaintext[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
	}

	return PKCS7Unpad(plaintext)
}

// ecbChunkSize is the ciphertext chunk size used by the streaming decrypter.
// It must be a multiple of aes.BlockSize.
const ecbChunkSize = 32 * 1024

// NewECBDecryptReader returns an io.Reader that streams the AES-128-ECB
// decryption of the ciphertext read from r, using constant memory.
//
// Each 16-byte block is decrypted independently. The final decrypted block is
// withheld until the upstream reader returns io.EOF, at which point PKCS7
// padding is removed and the remaining plaintext is emitted.
//
// Errors are reported through Read:
//   - key is not 16 bytes
//   - empty ciphertext
//   - total ciphertext length not a multiple of 16
//   - invalid PKCS7 padding
//
// Errors from the upstream reader (other than io.EOF) are propagated as-is.
func NewECBDecryptReader(r io.Reader, key []byte) io.Reader {
	if len(key) != 16 {
		return &ecbDecryptReader{err: fmt.Errorf("AES key must be 16 bytes, got %d", len(key))}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return &ecbDecryptReader{err: fmt.Errorf("create cipher: %w", err)}
	}
	return &ecbDecryptReader{
		src:   r,
		block: block,
		chunk: make([]byte, ecbChunkSize),
	}
}

// ecbDecryptReader implements the streaming AES-128-ECB decrypter.
type ecbDecryptReader struct {
	src      io.Reader
	block    cipher.Block
	chunk    []byte // scratch buffer for upstream reads
	raw      []byte // buffered ciphertext bytes (< aes.BlockSize after each fill)
	held     []byte // last decrypted block, withheld until upstream EOF
	out      []byte // decrypted plaintext ready to be served
	sawData  bool   // true once any ciphertext byte has been read
	finished bool   // true once the final (unpadded) block has been emitted
	err      error  // sticky error
}

func (d *ecbDecryptReader) Read(p []byte) (int, error) {
	for len(d.out) == 0 {
		if d.err != nil {
			return 0, d.err
		}
		if d.finished {
			return 0, io.EOF
		}
		d.fill()
	}
	n := copy(p, d.out)
	d.out = d.out[n:]
	return n, nil
}

// fill reads one chunk of ciphertext from upstream, decrypts all complete
// blocks (always withholding the last decrypted block), and on upstream EOF
// removes the PKCS7 padding from the withheld block.
func (d *ecbDecryptReader) fill() {
	n, readErr := d.src.Read(d.chunk)
	if n > 0 {
		d.sawData = true
		d.raw = append(d.raw, d.chunk[:n]...)
	}

	// Decrypt all complete blocks currently buffered.
	fullLen := (len(d.raw) / aes.BlockSize) * aes.BlockSize
	if fullLen > 0 {
		decrypted := make([]byte, fullLen)
		for i := 0; i < fullLen; i += aes.BlockSize {
			d.block.Decrypt(decrypted[i:i+aes.BlockSize], d.raw[i:i+aes.BlockSize])
		}
		d.raw = d.raw[fullLen:]

		// Emit previously withheld block plus all but the last new block;
		// withhold the last decrypted block until EOF is confirmed.
		d.out = append(d.out, d.held...)
		d.out = append(d.out, decrypted[:fullLen-aes.BlockSize]...)
		d.held = decrypted[fullLen-aes.BlockSize:]
	}

	if readErr == nil {
		return
	}
	if readErr != io.EOF {
		d.err = readErr
		return
	}

	// Upstream EOF: validate and finalize.
	switch {
	case !d.sawData:
		d.err = fmt.Errorf("empty ciphertext")
	case len(d.raw) != 0:
		d.err = fmt.Errorf("ciphertext length is not a multiple of block size %d: %d trailing bytes", aes.BlockSize, len(d.raw))
	default:
		unpadded, err := PKCS7Unpad(d.held)
		if err != nil {
			d.err = fmt.Errorf("invalid PKCS7 padding: %w", err)
			return
		}
		d.out = append(d.out, unpadded...)
		d.held = nil
		d.finished = true
	}
}
