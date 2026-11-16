package main

import (
	"io"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Unfield/Tern/kit/kittest"
)

// These tests run the whole app, as a browser would use it, against a
// database made for each test: npm test, or go tool tern build && go test ./...

func newClient(t *testing.T) *kittest.Client {
	t.Setenv("DATABASE_PATH", filepath.Join(t.TempDir(), "test.db"))
	server, err := newServer(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return kittest.NewClient(t, server)
}

func TestHomePage(t *testing.T) {
	client := newClient(t)
	if page := client.Get("/"); page.Status != 200 || !strings.Contains(page.Body, "open-source webmail") {
		t.Fatalf("home: %d", page.Status)
	}
}

func TestSignUpAndMail(t *testing.T) {
	client := newClient(t)
	if page := client.Get("/mail"); page.Status/100 != 3 || !strings.HasPrefix(page.Location(), "/login") {
		t.Fatalf("mail isn't protected: %d %s", page.Status, page.Location())
	}
	page := client.PostForm("/signup", url.Values{"name": {"Ada"}, "email": {"ada@example.com"}, "password": {"a long enough password"}})
	if page.Status != 303 {
		t.Fatalf("sign-up: %d", page.Status)
	}
	if page := client.Get("/mail"); page.Status != 200 || !strings.Contains(page.Body, "Your inbox lives here") {
		t.Fatalf("mail after sign-up: %d", page.Status)
	}
}
