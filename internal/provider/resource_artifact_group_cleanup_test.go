package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/fpetr/terraform-provider-apicurio/internal/client"
)

type fakeGroupCleanupClient struct {
	hasAnyResp bool
	hasAnyErr  *client.ResponseError

	deleteErr *client.ResponseError

	deleteCalls int
}

func (f *fakeGroupCleanupClient) GroupHasAnyArtifacts(ctx context.Context, groupID string) (bool, *client.ResponseError) {
	return f.hasAnyResp, f.hasAnyErr
}

func (f *fakeGroupCleanupClient) DeleteGroup(ctx context.Context, groupID string) *client.ResponseError {
	f.deleteCalls++
	return f.deleteErr
}

func TestDeleteGroupIfEmpty_whenNonEmpty_noDelete(t *testing.T) {
	f := &fakeGroupCleanupClient{hasAnyResp: true}
	deleted, err := deleteGroupIfEmpty(context.Background(), f, "g1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if deleted {
		t.Fatalf("expected deleted=false")
	}
	if f.deleteCalls != 0 {
		t.Fatalf("expected 0 delete calls, got %d", f.deleteCalls)
	}
}

func TestDeleteGroupIfEmpty_whenEmpty_deletes(t *testing.T) {
	f := &fakeGroupCleanupClient{hasAnyResp: false}
	deleted, err := deleteGroupIfEmpty(context.Background(), f, "g1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !deleted {
		t.Fatalf("expected deleted=true")
	}
	if f.deleteCalls != 1 {
		t.Fatalf("expected 1 delete call, got %d", f.deleteCalls)
	}
}

func TestDeleteGroupIfEmpty_whenCheckFails_returnsErr(t *testing.T) {
	f := &fakeGroupCleanupClient{hasAnyErr: &client.ResponseError{Err: errors.New("boom")}}
	deleted, err := deleteGroupIfEmpty(context.Background(), f, "g1")
	if err == nil {
		t.Fatalf("expected error")
	}
	if deleted {
		t.Fatalf("expected deleted=false")
	}
	if f.deleteCalls != 0 {
		t.Fatalf("expected 0 delete calls, got %d", f.deleteCalls)
	}
}

func TestDeleteGroupIfEmpty_whenDeleteFails_returnsErr(t *testing.T) {
	f := &fakeGroupCleanupClient{deleteErr: &client.ResponseError{Err: errors.New("boom")}}
	deleted, err := deleteGroupIfEmpty(context.Background(), f, "g1")
	if err == nil {
		t.Fatalf("expected error")
	}
	if deleted {
		t.Fatalf("expected deleted=false")
	}
	if f.deleteCalls != 1 {
		t.Fatalf("expected 1 delete call, got %d", f.deleteCalls)
	}
}
