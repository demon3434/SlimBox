package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type CLIDownloadHandler struct{}

func NewCLIDownloadHandler() *CLIDownloadHandler {
	return &CLIDownloadHandler{}
}

func (h *CLIDownloadHandler) findBinary(target string) (actualPath, downloadName string, err error) {
	target = strings.ToLower(strings.TrimSpace(target))

	var candidateFiles []string
	switch target {
	case "windows", "windows-amd64", "win", "win64":
		candidateFiles = []string{"slimbox-cli-windows-amd64.exe", "slimbox-cli.exe"}
		downloadName = "slimbox-cli.exe"
	case "darwin-arm64", "macos-arm64", "macos-m1", "macos-apple", "mac-arm64":
		candidateFiles = []string{"slimbox-cli-darwin-arm64"}
		downloadName = "slimbox-cli"
	case "darwin-amd64", "macos-amd64", "macos-intel", "mac-amd64":
		candidateFiles = []string{"slimbox-cli-darwin-amd64"}
		downloadName = "slimbox-cli"
	case "linux-arm64", "arm64", "linux-aarch64":
		candidateFiles = []string{"slimbox-cli-linux-arm64"}
		downloadName = "slimbox-cli"
	case "linux", "linux-amd64", "linux-x64", "linux64":
		candidateFiles = []string{"slimbox-cli-linux-amd64", "slimbox-cli"}
		downloadName = "slimbox-cli"
	default:
		return "", "", fmt.Errorf("unsupported target platform: %s", target)
	}

	searchDirs := []string{
		"/app/cli-dist",
		"./cli-dist",
		"cli-dist",
		"/data/cli-dist",
	}

	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		searchDirs = append(searchDirs,
			filepath.Join(execDir, "cli-dist"),
			filepath.Join(execDir, "..", "Resources", "cli-dist"),
		)
	}

	for _, dir := range searchDirs {
		for _, file := range candidateFiles {
			p := filepath.Join(dir, file)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p, downloadName, nil
			}
		}
	}

	return "", "", fmt.Errorf("cli binary for %s is not available on server", target)
}

// HandleDownload serves the pre-compiled standalone binary for the requested client OS/arch.
func (h *CLIDownloadHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	target := r.URL.Query().Get("target")
	if target == "" {
		target = "windows"
	}

	actualPath, downloadName, err := h.findBinary(target)
	if err != nil {
		WriteJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, actualPath)
}
