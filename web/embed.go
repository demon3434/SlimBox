package web

import "embed"

// Assets embeds all static files (index.html, style.css, app.js).
//
//go:embed index.html style.css app.js
var Assets embed.FS
