package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestShortenHandler(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{"success", `{"url":"https://example.com/long/path?q=1#part"}`, 200},
		{"http URL", `{"url":"http://example.com"}`, 200},
		{"empty body", "", 400},
		{"malformed JSON", `{"url":`, 400},
		{"missing field", `{}`, 400},
		{"null object", `null`, 400},
		{"null URL", `{"url":null}`, 400},
		{"wrong type", `{"url":42}`, 400},
		{"array", `[]`, 400},
		{"invalid URL", `{"url":"not-a-url"}`, 400},
		{"unknown field", `{"url":"https://example.com","extra":1}`, 400},
		{"second object", `{"url":"https://example.com"} {}`, 400},
		{"trailing garbage", `{"url":"https://example.com"} broken`, 400},
		{"trailing whitespace", "{\"url\":\"https://example.com\"}\n\t", 200},
		{"oversized body", `{"url":"https://example.com/` + strings.Repeat("x", maxRequestBytes) + `"}`, 413},
		{"oversized trailing whitespace", `{"url":"https://example.com"}` + strings.Repeat(" ", maxRequestBytes), 413},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := NewURLShortener()
			request := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			newHandler(s).ServeHTTP(w, request)
			if w.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tt.status, w.Body)
			}
			if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
				t.Fatal("expected JSON")
			}
			if tt.status == 200 {
				var got shortenResponse
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				var input struct {
					URL string `json:"url"`
				}
				if err := json.Unmarshal([]byte(tt.body), &input); err != nil {
					t.Fatal(err)
				}
				original, err := s.GetOriginal(got.ShortURL)
				if !idPattern.MatchString(got.ShortURL) || got.OriginalURL != input.URL || original != input.URL || err != nil {
					t.Fatalf("response=%+v original=%q err=%v", got, original, err)
				}
			} else {
				var got map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got["error"] == "" {
					t.Fatalf("invalid error: %s", w.Body)
				}
				if len(s.urls) != 0 {
					t.Fatal("invalid request saved a URL")
				}
			}
		})
	}
}

func TestHandlerRoutes(t *testing.T) {
	s := NewURLShortener()
	s.urls["known123"] = "https://example.com/path?q=1#part"
	for _, tt := range []struct {
		name, method, path string
		status             int
		allow              string
	}{
		{"redirect", "GET", "/known123", 302, ""},
		{"unknown ID", "GET", "/missing1", 404, ""},
		{"missing ID", "GET", "/", 404, ""},
		{"nested path", "GET", "/known123/extra", 404, ""},
		{"shorten wrong method", "GET", "/shorten", 405, "POST"},
		{"redirect wrong method", "POST", "/known123", 405, "GET"},
		{"delete", "DELETE", "/known123", 405, "GET"},
		{"head", "HEAD", "/known123", 405, "GET"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			newHandler(s).ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			if w.Code != tt.status || w.Header().Get("Allow") != tt.allow {
				t.Fatalf("status=%d headers=%v", w.Code, w.Header())
			}
			if tt.status == 302 && w.Header().Get("Location") != s.urls["known123"] {
				t.Fatalf("Location=%q", w.Header().Get("Location"))
			}
		})
	}
}

func TestShortenHandler_InternalError(t *testing.T) {
	s := NewURLShortener()
	s.newID = func() (string, error) { return "", errors.New("private failure details") }
	w := httptest.NewRecorder()
	newHandler(s).ServeHTTP(w, httptest.NewRequest("POST", "/shorten", strings.NewReader(`{"url":"https://example.com"}`)))
	if w.Code != 500 || strings.Contains(w.Body.String(), "private failure") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
}

func TestHTTPRoundTrip(t *testing.T) {
	server := httptest.NewServer(newHandler(NewURLShortener()))
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	original := "https://example.com/long/path?q=value#fragment"
	response, err := client.Post(server.URL+"/shorten", "application/json", strings.NewReader(`{"url":"`+original+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var got shortenResponse
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || got.OriginalURL != original {
		t.Fatalf("response=%+v status=%d", got, response.StatusCode)
	}
	redirect, err := client.Get(server.URL + "/" + got.ShortURL)
	if err != nil {
		t.Fatal(err)
	}
	defer redirect.Body.Close()
	_, _ = io.Copy(io.Discard, redirect.Body)
	if redirect.StatusCode != 302 || redirect.Header.Get("Location") != original {
		t.Fatalf("status=%d Location=%q", redirect.StatusCode, redirect.Header.Get("Location"))
	}
}
