package main

import "testing"

func TestDocumentMimeType(t *testing.T) {
	cases := map[string]string{
		"/tmp/acc/Form.PDF":  "application/pdf",
		"/tmp/a/report.xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"/tmp/a/notes.txt":   "text/plain",
		"/tmp/a/blob":        "application/octet-stream",
	}
	for path, want := range cases {
		if got := documentMimeType(path); got != want {
			t.Errorf("documentMimeType(%q) = %q, want %q", path, got, want)
		}
	}
}
