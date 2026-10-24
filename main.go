package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const maxRequestBytes = 1 << 20

type shortenResponse struct {
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}

func newHandler(shortener *URLShortener) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/shorten" {
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			shortenHandler(w, r, shortener)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/")
		if id == "" || strings.Contains(id, "/") {
			writeError(w, http.StatusNotFound, ErrNotFound.Error())
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		original, err := shortener.GetOriginal(id)
		if err != nil {
			writeError(w, http.StatusNotFound, ErrNotFound.Error())
			return
		}
		http.Redirect(w, r, original, http.StatusFound)
	})
}

func shortenHandler(w http.ResponseWriter, r *http.Request, shortener *URLShortener) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	var request struct {
		URL string `json:"url"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&request)
	if err == nil {
		var extra any
		if nextErr := decoder.Decode(&extra); nextErr != io.EOF {
			err = nextErr
			if err == nil {
				err = errors.New("multiple JSON values")
			}
		}
	}
	if err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "expected one JSON object with a string url field")
		}
		return
	}
	id, err := shortener.Shorten(request.URL)
	if errors.Is(err, ErrInvalidURL) {
		writeError(w, http.StatusBadRequest, ErrInvalidURL.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not shorten URL")
		return
	}
	writeJSON(w, http.StatusOK, shortenResponse{ShortURL: id, OriginalURL: request.URL})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           newHandler(NewURLShortener()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("URL shortener listening on %s", addr)
	log.Fatal(server.ListenAndServe())
}
