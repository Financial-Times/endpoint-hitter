package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestExecuteHTTPRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Expected Authorization header to be 'Bearer test-token'")
		}
		if r.Header.Get("X-Request-Id") == "" {
			t.Error("Expected X-Request-Id header to be set")
		}
		_, _ = io.WriteString(w, `{"status": "ok"}`)
	}))
	defer server.Close()

	status, tid, err := executeHTTPRequest(server.URL, "GET", "Bearer test-token")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d", status)
	}
	if tid == "" {
		t.Error("Expected transaction ID to be set")
	}
}

func TestExecuteHTTPRequest_NonOKStatus(t *testing.T) {
	// Server returns 500 Internal Server Error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "something went wrong", http.StatusInternalServerError)
	}))
	defer server.Close()

	status, tid, err := executeHTTPRequest(server.URL, "GET", "Bearer test-token")
	if err == nil {
		t.Error("Expected error for non-200 response, got nil")
	}
	if status != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", status)
	}
	if tid == "" {
		t.Error("Expected transaction ID to be set")
	}
}

func TestExecuteHTTPRequest_InvalidURL(t *testing.T) {
	status, tid, err := executeHTTPRequest("http://%%%", "GET", "Bearer test-token")
	if err == nil {
		t.Error("Expected error due to malformed URL, got nil")
	}
	if status != http.StatusInternalServerError {
		t.Errorf("Expected internal server error status, got %d", status)
	}
	if tid == "" {
		t.Error("Expected transaction ID to be set")
	}
}

func TestExecuteHTTPRequest_ClientDoError(t *testing.T) {
	// Non-routable IP address that causes a connection error
	badURL := "http://192.0.2.1:12345" // Reserved for documentation

	status, tid, err := executeHTTPRequest(badURL, "GET", "Bearer test-token")
	if err == nil {
		t.Error("Expected error from client.Do, got nil")
	}
	if status != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", status)
	}
	if tid == "" {
		t.Error("Expected transaction ID to be set")
	}
}

func TestHitEndpoint_Success(t *testing.T) {
	// Override the httpExecutor to always return success for the test
	httpExecutor = func(_, _, _ string) (int, string, error) {
		return http.StatusOK, "tid_dummy", nil
	}

	uuids := []string{"uuid1", "uuid2", "uuid3"}
	successCh := make(chan struct{}, len(uuids))

	hitEndpoint("http://fake.com/resource/{uuid}", "GET", "user", "pass", uuids, 2, successCh)

	// Count the number of successes received
	count := 0
	for range successCh {
		count++
	}

	if count != len(uuids) {
		t.Errorf("Expected %d successful hits, got %d", len(uuids), count)
	}
}

func TestHitEndpoint_WithRetry(t *testing.T) {
	var lock sync.Mutex
	callCounts := make(map[string]int)

	// Custom httpExecutor to simulate retries
	httpExecutor = func(urlStr, _, _ string) (int, string, error) {
		lock.Lock()
		defer lock.Unlock()

		callCounts[urlStr]++
		if callCounts[urlStr] < 2 {
			return http.StatusServiceUnavailable, "tid_retry", http.ErrHandlerTimeout
		}
		return http.StatusOK, "tid_ok", nil
	}

	uuids := []string{"uuid4"}
	successCh := make(chan struct{}, 1)

	start := time.Now()
	hitEndpoint("http://fake.com/resource/{uuid}", "GET", "user", "pass", uuids, 1, successCh)
	duration := time.Since(start)

	// Check that the retry happened (takes at least 3s due to time.Sleep)
	if duration < 3*time.Second {
		t.Errorf("Expected at least 3 seconds due to retry delay, got %v", duration)
	}

	count := 0
	for range successCh {
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 successful retry, got %d", count)
	}
}
