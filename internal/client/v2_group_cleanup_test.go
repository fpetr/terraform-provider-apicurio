package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestV2Client_GroupHasAnyArtifacts_limit1_and_parse(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/apis/registry/v2/groups/g1/artifacts" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.URL.Query().Get("limit"); got != "1" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("bad limit"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{}]`))
	}))
	defer srv.Close()

	c := &v2Client{endpoint: srv.URL, httpClient: srv.Client(), cfg: ClientConfig{Endpoint: srv.URL}}
	ok, err := c.GroupHasAnyArtifacts(context.Background(), "g1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestV2Client_GroupHasAnyArtifacts_parseObjectShape(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"artifacts":[],"count":0}`))
		}))
		defer srv.Close()

		c := &v2Client{endpoint: srv.URL, httpClient: srv.Client(), cfg: ClientConfig{Endpoint: srv.URL}}
		ok, err := c.GroupHasAnyArtifacts(context.Background(), "g1")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if ok {
			t.Fatalf("expected ok=false")
		}
	})

	t.Run("non-empty", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"artifacts":[{"id":"a1"}],"count":1}`))
		}))
		defer srv.Close()

		c := &v2Client{endpoint: srv.URL, httpClient: srv.Client(), cfg: ClientConfig{Endpoint: srv.URL}}
		ok, err := c.GroupHasAnyArtifacts(context.Background(), "g1")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !ok {
			t.Fatalf("expected ok=true")
		}
	})

	t.Run("count-only", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":2}`))
		}))
		defer srv.Close()

		c := &v2Client{endpoint: srv.URL, httpClient: srv.Client(), cfg: ClientConfig{Endpoint: srv.URL}}
		ok, err := c.GroupHasAnyArtifacts(context.Background(), "g1")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !ok {
			t.Fatalf("expected ok=true")
		}
	})

	t.Run("malformed-artifacts-field", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"artifacts":{},"count":0}`))
		}))
		defer srv.Close()

		c := &v2Client{endpoint: srv.URL, httpClient: srv.Client(), cfg: ClientConfig{Endpoint: srv.URL}}
		ok, err := c.GroupHasAnyArtifacts(context.Background(), "g1")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if ok {
			t.Fatalf("expected ok=false")
		}
	})
}

func TestV2Client_GroupHasAnyArtifacts_404_is_empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &v2Client{endpoint: srv.URL, httpClient: srv.Client(), cfg: ClientConfig{Endpoint: srv.URL}}
	ok, err := c.GroupHasAnyArtifacts(context.Background(), "g1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false")
	}
}

func TestV2Client_DeleteGroup_404_and_409_are_noop(t *testing.T) {
	codes := []int{http.StatusNotFound, http.StatusConflict}
	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				w.WriteHeader(code)
			}))
			defer srv.Close()

			c := &v2Client{endpoint: srv.URL, httpClient: srv.Client(), cfg: ClientConfig{Endpoint: srv.URL}}
			if err := c.DeleteGroup(context.Background(), "g1"); err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}
