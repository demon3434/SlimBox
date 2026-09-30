package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// FindExecutable searches for the given tool binary (e.g. "ffmpeg", "ffprobe")
// prioritizing self-contained bundle directories before falling back to system PATH.
func FindExecutable(name string) string {
	binName := name
	if runtime.GOOS == "windows" && filepath.Ext(binName) == "" {
		binName += ".exe"
	}

	var candidates []string

	// 1. Check relative to the current running executable
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		// macOS .app bundle: SlimBox.app/Contents/MacOS/slimbox -> SlimBox.app/Contents/Resources/ffmpeg
		candidates = append(candidates, filepath.Join(execDir, "..", "Resources", binName))
		// Adjacent standalone bin folder: ./bin/ffmpeg
		candidates = append(candidates, filepath.Join(execDir, "bin", binName))
		// Adjacent root: ./ffmpeg
		candidates = append(candidates, filepath.Join(execDir, binName))
	}

	// 2. Check current working directory and parent paths
	candidates = append(candidates,
		filepath.Join(".", "bin", binName),
		filepath.Join(".", binName),
		filepath.Join("..", "bin", binName),
		filepath.Join("..", "..", "bin", binName),
	)

	// 3. Check standard macOS package manager directories (Homebrew on Apple Silicon & Intel)
	candidates = append(candidates,
		filepath.Join("/opt/homebrew/bin", binName),
		filepath.Join("/usr/local/bin", binName),
	)

	for _, path := range candidates {
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			// On Unix, ensure it is executable
			if runtime.GOOS != "windows" && fi.Mode()&0111 == 0 {
				continue
			}
			return path
		}
	}

	// 3. Fallback to system PATH lookup
	if path, err := exec.LookPath(name); err == nil {
		return path
	}

	return name
}

// FindFFmpeg returns the resolved path for the FFmpeg binary.
func FindFFmpeg() string {
	return FindExecutable("ffmpeg")
}

// FindFFprobe returns the resolved path for the FFprobe binary.
func FindFFprobe() string {
	return FindExecutable("ffprobe")
}
