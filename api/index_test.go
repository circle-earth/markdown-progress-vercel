package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "/progress/75", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(Handler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "image/svg+xml" {
		t.Errorf("handler returned wrong content type: got %v want %v", contentType, "image/svg+xml")
	}

	body := rr.Body.String()
	if !strings.Contains(body, "<svg") || !strings.Contains(body, "75%") {
		t.Errorf("handler body does not contain expected SVG content: %s", body)
	}
}
