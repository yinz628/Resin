package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Resinat/Resin/internal/service"
)

func TestWriteServiceError_InternalIncludesWrappedCause(t *testing.T) {
	rec := httptest.NewRecorder()

	writeServiceError(rec, &service.ServiceError{
		Code:    "INTERNAL",
		Message: "egress probe failed",
		Err:     errors.New("i/o timeout"),
	})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}

	if body.Error.Code != "INTERNAL" {
		t.Fatalf("error.code = %q, want %q", body.Error.Code, "INTERNAL")
	}
	if body.Error.Message != "egress probe failed: i/o timeout" {
		t.Fatalf("error.message = %q, want %q", body.Error.Message, "egress probe failed: i/o timeout")
	}
}

func TestWriteServiceError_InternalDoesNotDuplicateWrappedCause(t *testing.T) {
	rec := httptest.NewRecorder()

	writeServiceError(rec, &service.ServiceError{
		Code:    "INTERNAL",
		Message: "egress probe failed: i/o timeout",
		Err:     errors.New("i/o timeout"),
	})

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}

	if body.Error.Message != "egress probe failed: i/o timeout" {
		t.Fatalf("error.message = %q, want %q", body.Error.Message, "egress probe failed: i/o timeout")
	}
}

func TestWriteServiceError_InternalPrefersWrappedCauseWithExistingPrefix(t *testing.T) {
	rec := httptest.NewRecorder()

	writeServiceError(rec, &service.ServiceError{
		Code:    "INTERNAL",
		Message: "egress probe failed",
		Err:     errors.New(`egress probe failed: Get "https://cloudflare.com/cdn-cgi/trace": context deadline exceeded`),
	})

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}

	want := `egress probe failed: Get "https://cloudflare.com/cdn-cgi/trace": context deadline exceeded`
	if body.Error.Message != want {
		t.Fatalf("error.message = %q, want %q", body.Error.Message, want)
	}
}

func TestWriteServiceError_NonInternalKeepsOriginalMessage(t *testing.T) {
	rec := httptest.NewRecorder()

	writeServiceError(rec, &service.ServiceError{
		Code:    "INVALID_ARGUMENT",
		Message: "node_hash: invalid format",
		Err:     errors.New("should not be exposed"),
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}

	if body.Error.Message != "node_hash: invalid format" {
		t.Fatalf("error.message = %q, want %q", body.Error.Message, "node_hash: invalid format")
	}
}
