package main

import (
	"mime"
	"path/filepath"
	"strings"
)

// documentMimeType returns the MIME type for a document sent through send_file. A generic
// application/octet-stream makes WhatsApp clients show ".bin" and refuse to open a PDF.
func documentMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	known := map[string]string{
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".ppt":  "application/vnd.ms-powerpoint",
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".txt":  "text/plain",
		".csv":  "text/csv",
		".zip":  "application/zip",
	}
	if m, ok := known[ext]; ok {
		return m
	}
	if m := mime.TypeByExtension(ext); m != "" {
		return m
	}
	return "application/octet-stream"
}
