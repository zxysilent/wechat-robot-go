package model

import "testing"

func TestFileItemDeclaredSize(t *testing.T) {
	tests := []struct {
		name   string
		length string
		want   int64
	}{
		{"empty string", "", 0},
		{"non-numeric", "abc", 0},
		{"mixed", "123abc", 0},
		{"negative", "-5", 0},
		{"zero", "0", 0},
		{"valid", "12345", 12345},
		{"large", "9223372036854775807", 9223372036854775807},
		{"overflow", "92233720368547758080", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &FileItem{Length: tt.length}
			if got := f.DeclaredSize(); got != tt.want {
				t.Errorf("FileItem{Length: %q}.DeclaredSize() = %d, want %d", tt.length, got, tt.want)
			}
		})
	}
}

func TestVoiceItemDeclaredSize(t *testing.T) {
	if got := (&VoiceItem{FileSize: 4096}).DeclaredSize(); got != 4096 {
		t.Errorf("DeclaredSize() = %d, want 4096", got)
	}
	if got := (&VoiceItem{}).DeclaredSize(); got != 0 {
		t.Errorf("DeclaredSize() = %d, want 0", got)
	}
}

func TestVideoItemDeclaredSize(t *testing.T) {
	if got := (&VideoItem{VideoSize: 1 << 20}).DeclaredSize(); got != 1<<20 {
		t.Errorf("DeclaredSize() = %d, want %d", got, 1<<20)
	}
	if got := (&VideoItem{}).DeclaredSize(); got != 0 {
		t.Errorf("DeclaredSize() = %d, want 0", got)
	}
}

func TestImageItemDeclaredSize(t *testing.T) {
	tests := []struct {
		name    string
		hdSize  int
		midSize int
		want    int64
	}{
		{"HD preferred over Mid", 2048, 1024, 2048},
		{"fallback to Mid when HD is 0", 0, 1024, 1024},
		{"both zero", 0, 0, 0},
		{"HD only", 4096, 0, 4096},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ImageItem{HDSize: tt.hdSize, MidSize: tt.midSize}
			if got := i.DeclaredSize(); got != tt.want {
				t.Errorf("ImageItem{HDSize: %d, MidSize: %d}.DeclaredSize() = %d, want %d",
					tt.hdSize, tt.midSize, got, tt.want)
			}
		})
	}
}
