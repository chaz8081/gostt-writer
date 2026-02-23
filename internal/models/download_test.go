package models

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "dst.txt")

	content := []byte("hello world")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile() error = %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading dst: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("copyFile() content = %q, want %q", got, content)
	}
}

func TestCopyDir(t *testing.T) {
	tmpDir := t.TempDir()

	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	// Create source directory structure
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("aaa"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "b.txt"), []byte("bbb"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir() error = %v", err)
	}

	// Verify files exist
	got, err := os.ReadFile(filepath.Join(dstDir, "a.txt"))
	if err != nil {
		t.Fatalf("reading a.txt: %v", err)
	}
	if string(got) != "aaa" {
		t.Errorf("a.txt content = %q, want %q", got, "aaa")
	}

	got, err = os.ReadFile(filepath.Join(dstDir, "sub", "b.txt"))
	if err != nil {
		t.Fatalf("reading sub/b.txt: %v", err)
	}
	if string(got) != "bbb" {
		t.Errorf("sub/b.txt content = %q, want %q", got, "bbb")
	}
}

func TestProgressWriter(t *testing.T) {
	tmpDir := t.TempDir()
	f, err := os.Create(filepath.Join(tmpDir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	pw := &progressWriter{
		writer: f,
		total:  100,
		label:  "test",
	}

	data := make([]byte, 50)
	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != 50 {
		t.Errorf("Write() n = %d, want 50", n)
	}
	if pw.written != 50 {
		t.Errorf("written = %d, want 50", pw.written)
	}
}
