package web

import (
	"bytes"
	"io/fs"
	"testing"
)

func TestEmbeddedAssets(t *testing.T) {
	requiredFiles := []string{
		"index.html",
		"style.css",
		"css/themes.css",
		"css/base.css",
		"css/header.css",
		"css/config.css",
		"css/tasks.css",
		"css/modals.css",
		"css/dialog.css",
		"logo.png",
		"js/main.js",
		"js/bus.js",
		"js/api.js",
		"js/utils.js",
		"js/modules/auth.js",
		"js/modules/stats.js",
		"js/modules/theme.js",
		"js/modules/upload.js",
		"js/modules/profiles.js",
		"js/modules/tasks.js",
		"js/modules/settings.js",
		"js/modules/dialog.js",
		"js/modules/guide.js",
		"img/os-windows.svg",
		"img/os-apple.svg",
		"img/os-linux.svg",
	}

	for _, file := range requiredFiles {
		data, err := fs.ReadFile(Assets, file)
		if err != nil {
			t.Errorf("expected embedded file %q not found or error: %v", file, err)
			continue
		} else if len(data) == 0 {
			t.Errorf("embedded file %q is empty", file)
			continue
		}

		// Verify file size limit: no JS file should exceed 500 lines
		if bytes.HasSuffix([]byte(file), []byte(".js")) {
			lines := bytes.Count(data, []byte("\n")) + 1
			t.Logf("File: %-25s | Lines: %3d", file, lines)
			if lines > 500 {
				t.Errorf("file %q exceeds 500-line constraint: has %d lines", file, lines)
			}
		}
	}
}
