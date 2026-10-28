package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recorder := httptest.NewRecorder()
	newRouter().ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var body map[string]string
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if len(body) != 1 || body["status"] != "ok" {
		t.Errorf("body = %v, want status:ok", body)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Errorf("expected end of JSON response, got %v", err)
	}
}
