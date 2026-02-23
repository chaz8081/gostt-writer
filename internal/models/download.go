package models

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chaz8081/gostt-writer/internal/config"
)

const (
	whisperModelURL  = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin"
	whisperModelName = "ggml-base.en.bin"
	parakeetRepo     = "https://huggingface.co/FluidInference/parakeet-tdt-0.6b-v2-coreml"
	parakeetDirName  = "parakeet-tdt-v2"
)

// parakeetFiles are the files needed from the parakeet HuggingFace repo.
var parakeetFiles = []string{
	"Preprocessor.mlmodelc",
	"Encoder.mlmodelc",
	"Decoder.mlmodelc",
	"JointDecision.mlmodelc",
	"parakeet_vocab.json",
}

// DownloadWhisper downloads the whisper ggml model to the default models directory.
// It shows download progress to stdout.
func DownloadWhisper() error {
	modelsDir := config.DefaultModelsDir()
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("creating models dir: %w", err)
	}

	destPath := filepath.Join(modelsDir, whisperModelName)

	// Check if already downloaded
	if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
		fmt.Printf("  Whisper model already exists: %s (%.0f MB)\n", destPath, float64(info.Size())/(1024*1024))
		return nil
	}

	fmt.Printf("  Downloading whisper model from HuggingFace...\n")
	fmt.Printf("  URL: %s\n", whisperModelURL)
	fmt.Printf("  Destination: %s\n", destPath)

	resp, err := http.Get(whisperModelURL) //nolint:gosec // URL is a compile-time constant
	if err != nil {
		return fmt.Errorf("downloading whisper model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Write to temp file first, then rename (atomic)
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	// Progress writer
	pr := &progressWriter{
		writer: f,
		total:  resp.ContentLength,
		label:  whisperModelName,
	}

	written, err := io.Copy(pr, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("writing model file: %w", err)
	}

	fmt.Printf("\n  Downloaded %.1f MB\n", float64(written)/(1024*1024))

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("moving model file: %w", err)
	}

	return nil
}

// DownloadParakeet downloads the parakeet CoreML models via git sparse-checkout.
// Requires git and git-lfs to be installed.
func DownloadParakeet() error {
	// Check prerequisites
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is required for parakeet download but not found in PATH")
	}
	if err := checkGitLFS(); err != nil {
		return fmt.Errorf("git-lfs is required for parakeet download: %w", err)
	}

	modelsDir := config.DefaultModelsDir()
	destDir := filepath.Join(modelsDir, parakeetDirName)

	// Check if already downloaded
	encoderPath := filepath.Join(destDir, "Encoder.mlmodelc")
	if _, err := os.Stat(encoderPath); err == nil {
		fmt.Printf("  Parakeet models already exist: %s\n", destDir)
		return nil
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating models dir: %w", err)
	}

	fmt.Printf("  Downloading parakeet models from HuggingFace...\n")
	fmt.Printf("  Repo: %s\n", parakeetRepo)
	fmt.Printf("  Destination: %s\n", destDir)
	fmt.Printf("  This may take a few minutes (CoreML models are large).\n")

	// Create temp dir for clone
	tmpDir, err := os.MkdirTemp("", "gostt-parakeet-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Sparse checkout + LFS
	cmds := []struct {
		name string
		args []string
		dir  string
	}{
		{"Cloning (sparse)...", []string{"git", "clone", "--filter=blob:none", "--no-checkout", parakeetRepo, tmpDir}, ""},
		{"Setting sparse-checkout...", []string{"git", "sparse-checkout", "set",
			"Preprocessor.mlmodelc", "Encoder.mlmodelc", "Decoder.mlmodelc",
			"JointDecision.mlmodelc", "parakeet_vocab.json"}, tmpDir},
		{"Checking out...", []string{"git", "checkout"}, tmpDir},
		{"Pulling LFS objects...", []string{"git", "lfs", "pull"}, tmpDir},
	}

	for _, c := range cmds {
		fmt.Printf("  %s\n", c.name)
		cmd := exec.Command(c.args[0], c.args[1:]...) //nolint:gosec // args are compile-time constants
		if c.dir != "" {
			cmd.Dir = c.dir
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", c.name, err)
		}
	}

	// Copy files to destination
	fmt.Printf("  Copying models to %s...\n", destDir)
	for _, name := range parakeetFiles {
		src := filepath.Join(tmpDir, name)
		dst := filepath.Join(destDir, name)
		if err := copyFileOrDir(src, dst); err != nil {
			return fmt.Errorf("copying %s: %w", name, err)
		}
	}

	fmt.Printf("  Parakeet models installed successfully.\n")
	return nil
}

// checkGitLFS verifies git-lfs is installed.
func checkGitLFS() error {
	cmd := exec.Command("git", "lfs", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git-lfs not found: install with 'brew install git-lfs && git lfs install'")
	}
	return nil
}

// copyFileOrDir copies a file or directory recursively.
func copyFileOrDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if err := copyFileOrDir(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// progressWriter wraps an io.Writer and prints download progress.
type progressWriter struct {
	writer  io.Writer
	total   int64
	written int64
	label   string
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	pw.written += int64(n)
	if pw.total > 0 {
		pct := float64(pw.written) / float64(pw.total) * 100
		fmt.Printf("\r  %s: %.1f MB / %.1f MB (%.0f%%)",
			pw.label,
			float64(pw.written)/(1024*1024),
			float64(pw.total)/(1024*1024),
			pct)
	} else {
		fmt.Printf("\r  %s: %.1f MB downloaded",
			pw.label,
			float64(pw.written)/(1024*1024))
	}
	return n, err
}

// RunInteractiveDownload runs the interactive model download flow.
// It prompts the user which models to download and downloads them.
func RunInteractiveDownload() error {
	fmt.Println("=== Model Download ===")
	fmt.Println()
	fmt.Printf("Models will be downloaded to: %s\n", config.DefaultModelsDir())
	fmt.Println()
	fmt.Println("Which models would you like to download?")
	fmt.Println("  [1] Whisper (base.en, ~142 MB) - CPU/GPU transcription")
	fmt.Println("  [2] Parakeet (TDT v2, ~1.2 GB) - Apple Neural Engine (faster, macOS only)")
	fmt.Println("  [3] Both")
	fmt.Println()
	fmt.Print("Choice [1/2/3]: ")

	var choice string
	fmt.Scanln(&choice)
	choice = strings.TrimSpace(choice)

	fmt.Println()

	switch choice {
	case "1":
		fmt.Println("Downloading Whisper model...")
		return DownloadWhisper()
	case "2":
		fmt.Println("Downloading Parakeet models...")
		return DownloadParakeet()
	case "3":
		fmt.Println("Downloading all models...")
		fmt.Println()
		fmt.Println("[1/2] Whisper model:")
		if err := DownloadWhisper(); err != nil {
			return fmt.Errorf("whisper download failed: %w", err)
		}
		fmt.Println()
		fmt.Println("[2/2] Parakeet models:")
		if err := DownloadParakeet(); err != nil {
			return fmt.Errorf("parakeet download failed: %w", err)
		}
		fmt.Println()
		fmt.Println("All models downloaded successfully!")
		return nil
	default:
		return fmt.Errorf("invalid choice: %q (expected 1, 2, or 3)", choice)
	}
}
