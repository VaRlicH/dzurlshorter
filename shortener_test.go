package main

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
	"testing"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8}$`)

func TestURLShortener_Shorten(t *testing.T) {
	tests := []struct {
		name, url string
		valid     bool
	}{
		{"http", "http://example.com", true},
		{"https with query and fragment", "https://example.com/a?q=test#section", true},
		{"encoded path", "https://example.com/a%20b", true},
		{"localhost", "http://localhost:8080/path", true},
		{"IPv6", "http://[::1]:8080/path", true},
		{"uppercase scheme", "HTTPS://example.com", true},
		{"empty", "", false},
		{"relative", "/path", false},
		{"missing scheme", "example.com/path", false},
		{"protocol relative", "//example.com", false},
		{"ftp", "ftp://example.com", false},
		{"javascript", "javascript:alert(1)", false},
		{"missing host", "https:///path", false},
		{"missing hostname", "http://:8080", false},
		{"opaque URL", "http:example.com", false},
		{"space in hostname", "http://exam ple.com", false},
		{"space in path", "http://example.com/a b", false},
		{"leading whitespace", " https://example.com", false},
		{"control characters", "https://example.com/\r\nLocation:evil", false},
		{"invalid escape", "https://example.com/%zz", false},
		{"invalid port", "http://example.com:abc", false},
		{"empty port", "http://example.com:", false},
		{"zero port", "http://example.com:0", false},
		{"port too large", "http://example.com:65536", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewURLShortener()
			id, err := s.Shorten(tt.url)
			if !tt.valid {
				if !errors.Is(err, ErrInvalidURL) || id != "" || len(s.urls) != 0 {
					t.Fatalf("invalid URL: id=%q err=%v stored=%d", id, err, len(s.urls))
				}
				return
			}
			if err != nil || !idPattern.MatchString(id) {
				t.Fatalf("id=%q err=%v", id, err)
			}
			original, err := s.GetOriginal(id)
			if err != nil || original != tt.url {
				t.Fatalf("original=%q err=%v", original, err)
			}
		})
	}
}

func TestURLShortener_GetOriginal(t *testing.T) {
	s := NewURLShortener()
	s.urls["known123"] = "https://example.com"
	for _, tt := range []struct {
		name, id, want string
		wantErr        error
	}{
		{"found", "known123", "https://example.com", nil},
		{"missing", "missing1", "", ErrNotFound},
		{"empty", "", "", ErrNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.GetOriginal(tt.id)
			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Fatalf("got=%q err=%v", got, err)
			}
		})
	}
}

func TestURLShortener_CollisionsAndErrors(t *testing.T) {
	t.Run("collision is retried without overwriting", func(t *testing.T) {
		s := NewURLShortener()
		s.urls["existing"] = "https://first.example"
		calls := 0
		s.newID = func() (string, error) {
			calls++
			if calls == 1 {
				return "existing", nil
			}
			return "new12345", nil
		}
		id, err := s.Shorten("https://second.example")
		if err != nil || id != "new12345" || calls != 2 || s.urls["existing"] != "https://first.example" {
			t.Fatalf("id=%q err=%v calls=%d urls=%v", id, err, calls, s.urls)
		}
	})
	t.Run("collision limit", func(t *testing.T) {
		s := NewURLShortener()
		s.urls["existing"] = "https://first.example"
		calls := 0
		s.newID = func() (string, error) { calls++; return "existing", nil }
		id, err := s.Shorten("https://second.example")
		if !errors.Is(err, ErrIDExhausted) || id != "" || calls != maxIDAttempts || len(s.urls) != 1 {
			t.Fatalf("id=%q err=%v calls=%d", id, err, calls)
		}
	})
	t.Run("generator error", func(t *testing.T) {
		s := NewURLShortener()
		failure := errors.New("random source failed")
		s.newID = func() (string, error) { return "", failure }
		id, err := s.Shorten("https://example.com")
		if !errors.Is(err, failure) || id != "" || len(s.urls) != 0 {
			t.Fatalf("id=%q err=%v", id, err)
		}
	})
}

func TestURLShortener_Concurrent(t *testing.T) {
	s := NewURLShortener()
	const count = 200
	ids := make(chan string, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			original := fmt.Sprintf("https://example.com/%d", i)
			id, err := s.Shorten(original)
			if err != nil {
				t.Errorf("Shorten: %v", err)
				return
			}
			got, err := s.GetOriginal(id)
			if err != nil || got != original {
				t.Errorf("GetOriginal=%q err=%v", got, err)
			}
			ids <- id
		}(i)
	}
	wg.Wait()
	close(ids)
	seen := make(map[string]bool)
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate ID: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != count {
		t.Fatalf("stored %d IDs, want %d", len(seen), count)
	}
}

func TestGenerateShortID(t *testing.T) {
	for i := 0; i < 100; i++ {
		id, err := generateShortID()
		if err != nil || !idPattern.MatchString(id) {
			t.Fatalf("id=%q err=%v", id, err)
		}
	}
}
