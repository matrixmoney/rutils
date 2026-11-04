package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"file-converter/internal/format"
)

// HandleFormat implements POST /format.
//
// Accepts multipart/form-data:
//   file         (required) — the raw bytes to format
//   format       (optional) — force a source format ("html", "json", ...).
//                             When empty, the engine detects it.
//   describe     (optional) — "true" to return a JSON envelope. When
//                             absent or "false", the formatted bytes are
//                             returned directly.
//
// The endpoint ALWAYS returns 200 on any input that is not zero bytes,
// because formatting is best-effort by design. The only 4xx responses
// are:
//
//   - 400 when the file field is missing or the upload is empty
//   - 415 when the input cannot be identified as any supported format
//   - 405 when the method is not POST
func HandleFormat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	// 10 MB cap, matching the existing /repair handler.
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "Failed to parse multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required: "+err.Error())
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Failed to read uploaded file: "+err.Error())
		return
	}
	if len(raw) == 0 {
		writeError(w, http.StatusBadRequest, "input is empty")
		return
	}

	// Filename: drives DetectFormat's extension fallback. When the
	// caller forces a format via the "format" field, we spoof the
	// matching extension so the extension lookup wins.
	filename := ""
	if header != nil {
		filename = header.Filename
	}
	if forced := strings.ToLower(strings.TrimSpace(r.FormValue("format"))); forced != "" {
		filename = "upload." + forced
	}

	eng := format.NewEngine()
	out, report, err := eng.Format(filename, raw)
	if err != nil {
		switch err {
		case format.ErrUnsupportedFormat:
			writeError(w, http.StatusUnsupportedMediaType, err.Error())
		case format.ErrEmptyInput:
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			log.Printf("[format] unexpected error: %v", err)
			writeError(w, http.StatusInternalServerError, "Format failed: "+err.Error())
		}
		return
	}

	// Always set the compact header summary so clients on the raw
	// path can read it without parsing the body.
	w.Header().Set("X-Format-Applied", report.HeaderValue())
	w.Header().Set("X-Format-Format", report.Format)
	if report.Changed {
		w.Header().Set("X-Format-Changed", "true")
	} else {
		w.Header().Set("X-Format-Changed", "false")
	}
	w.Header().Set("Access-Control-Expose-Headers",
		"X-Format-Applied, X-Format-Format, X-Format-Changed")

	// describe=true → JSON envelope
	if strings.EqualFold(strings.TrimSpace(r.FormValue("describe")), "true") {
		w.Header().Set("Content-Type", "application/json")

		applied := report.Applied
		if applied == nil {
			applied = []string{}
		}
		warnings := report.Warnings
		if warnings == nil {
			warnings = []string{}
		}
		envelope := map[string]any{
			"format":   report.Format,
			"changed":  report.Changed,
			"applied":  applied,
			"warnings": warnings,
			"output":   string(out),
		}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(envelope); err != nil {
			log.Printf("[format] failed to encode JSON envelope: %v", err)
		}
		return
	}

	// Raw bytes path (describe absent/false)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition",
		`attachment; filename="formatted.`+report.Format+`"`)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(out); err != nil {
		log.Printf("[format] failed to write formatted bytes: %v", err)
	}
}