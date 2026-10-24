package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

var (
	ErrInvalidURL  = errors.New("url must be a valid HTTP or HTTPS address")
	ErrNotFound    = errors.New("short URL not found")
	ErrIDExhausted = errors.New("could not generate a unique short ID")
)

const maxIDAttempts = 16

// URLShortener stores URLs in memory and is safe for concurrent use.
// Create instances with NewURLShortener.
type URLShortener struct {
	urls  map[string]string
	mu    sync.RWMutex
	newID func() (string, error)
}

func NewURLShortener() *URLShortener {
	return &URLShortener{urls: make(map[string]string), newID: generateShortID}
}

// Shorten validates and stores a URL without changing its original spelling.
func (us *URLShortener) Shorten(originalURL string) (string, error) {
	if !isValidURL(originalURL) {
		return "", ErrInvalidURL
	}
	for attempt := 0; attempt < maxIDAttempts; attempt++ {
		id, err := us.newID()
		if err != nil {
			return "", fmt.Errorf("generate short ID: %w", err)
		}
		us.mu.Lock()
		_, exists := us.urls[id]
		if !exists {
			us.urls[id] = originalURL
		}
		us.mu.Unlock()
		if !exists {
			return id, nil
		}
	}
	return "", ErrIDExhausted
}

func (us *URLShortener) GetOriginal(shortID string) (string, error) {
	us.mu.RLock()
	original, ok := us.urls[shortID]
	us.mu.RUnlock()
	if !ok {
		return "", ErrNotFound
	}
	return original, nil
}

func generateShortID() (string, error) {
	var data [6]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data[:]), nil
}

func isValidURL(raw string) bool {
	if strings.ContainsFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	if strings.HasSuffix(u.Host, ":") {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}
