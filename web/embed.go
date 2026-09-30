package web

import "embed"

// Assets embeds all static files (index.html, style.css, js/...).
//
//go:embed index.html style.css js logo.png img/* css/*
var Assets embed.FS
