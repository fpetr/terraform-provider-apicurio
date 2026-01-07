package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestV2CreateArtifactVersion_SendsVersionHeader(t *testing.T) {
	var gotVersion string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get("X-Registry-Version")
		w.WriteHeader(http.StatusOK)
		// decodeMetaJSON accepts empty body.
	}))
	defer srv.Close()

	c := NewV2(srv.URL, srv.Client(), ClientConfig{Endpoint: srv.URL, APIVersion: ServerFlavorV2})
	v := "v2"
	if _, err := c.CreateArtifactVersion(context.Background(), "g", "a", &v, []byte("x")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotVersion != "v2" {
		t.Fatalf("expected X-Registry-Version=v2, got %q", gotVersion)
	}
}

func TestV2CreateArtifactVersion_OmitsVersionHeaderWhenNil(t *testing.T) {
	var gotVersion string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get("X-Registry-Version")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewV2(srv.URL, srv.Client(), ClientConfig{Endpoint: srv.URL, APIVersion: ServerFlavorV2})
	if _, err := c.CreateArtifactVersion(context.Background(), "g", "a", nil, []byte("x")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotVersion != "" {
		t.Fatalf("expected no X-Registry-Version header, got %q", gotVersion)
	}
}

func TestV2CreateArtifact_SendsVersionHeader(t *testing.T) {
	var gotVersion string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get("X-Registry-Version")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewV2(srv.URL, srv.Client(), ClientConfig{Endpoint: srv.URL, APIVersion: ServerFlavorV2})
	v := "v1"
	if _, err := c.CreateArtifact(context.Background(), "g", "a", "AVRO", &v, []byte("x")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotVersion != "v1" {
		t.Fatalf("expected X-Registry-Version=v1, got %q", gotVersion)
	}
}
