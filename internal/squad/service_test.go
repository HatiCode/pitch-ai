package squad

import (
	"context"
	"errors"
	"testing"

	"pitch-ai/internal/core"
)

func samplePlayer() core.Player {
	return core.Player{
		FirstName: "Thomas",
		LastName:  "Lefevre",
		Positions: []core.Position{core.Openside},
		Status:    core.PlayerActive,
	}
}

// newService wires a service to a fresh store with a predictable ID generator.
func newService() (*Service, *memStore) {
	store := newMemStore()
	return NewService(store, store, func() string { return "generated-id-a" }), store
}

func TestAddAssignsIDAndClub(t *testing.T) {
	svc, _ := newService()

	got, err := svc.Add(context.Background(), "club-1", samplePlayer())
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got.ID != "generated-id-a" {
		t.Errorf("ID = %q, want the generated id", got.ID)
	}
	if got.ClubID != "club-1" {
		t.Errorf("ClubID = %q, want club-1", got.ClubID)
	}
}

func TestAddIgnoresCallerSuppliedIDAndClub(t *testing.T) {
	svc, _ := newService()

	p := samplePlayer()
	p.ID = "attacker-chosen-id"
	p.ClubID = "someone-elses-club"

	got, err := svc.Add(context.Background(), "club-1", p)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got.ID != "generated-id-a" {
		t.Errorf("ID = %q, want the service-generated id", got.ID)
	}
	if got.ClubID != "club-1" {
		t.Errorf("ClubID = %q, want the caller's club, not the body's", got.ClubID)
	}
}

func TestAddRejectsInvalidPlayer(t *testing.T) {
	svc, store := newService()

	p := samplePlayer()
	p.LastName = ""

	if _, err := svc.Add(context.Background(), "club-1", p); err == nil {
		t.Fatal("Add() error = nil, want a validation error")
	}
	if len(store.players["club-1"]) != 0 {
		t.Error("an invalid player was persisted")
	}
}

func TestAddDefaultsStatusToActive(t *testing.T) {
	svc, _ := newService()

	p := samplePlayer()
	p.Status = ""

	got, err := svc.Add(context.Background(), "club-1", p)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got.Status != core.PlayerActive {
		t.Errorf("Status = %q, want %q", got.Status, core.PlayerActive)
	}
}

func TestGetDoesNotLeakAcrossClubs(t *testing.T) {
	svc, _ := newService()

	added, err := svc.Add(context.Background(), "club-1", samplePlayer())
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if _, err := svc.Get(context.Background(), "club-2", added.ID); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Get() from another club error = %v, want core.ErrNotFound", err)
	}
}

func TestUpdatePreservesIdentity(t *testing.T) {
	svc, _ := newService()

	added, err := svc.Add(context.Background(), "club-1", samplePlayer())
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	changed := samplePlayer()
	changed.LastName = "Lefevre-Martin"
	changed.ID = "hijack"
	changed.ClubID = "club-2"

	got, err := svc.Update(context.Background(), "club-1", added.ID, changed)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.ID != added.ID || got.ClubID != "club-1" {
		t.Errorf("identity = (%q, %q), want (%q, club-1)", got.ID, got.ClubID, added.ID)
	}
	if got.LastName != "Lefevre-Martin" {
		t.Errorf("LastName = %q, want the updated value", got.LastName)
	}
}

func TestUpdateUnknownPlayerIsNotFound(t *testing.T) {
	svc, _ := newService()

	_, err := svc.Update(context.Background(), "club-1", "nope", samplePlayer())
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Update() error = %v, want core.ErrNotFound", err)
	}
}

func TestUpdateCannotReachIntoAnotherClub(t *testing.T) {
	svc, _ := newService()

	added, err := svc.Add(context.Background(), "club-1", samplePlayer())
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	_, err = svc.Update(context.Background(), "club-2", added.ID, samplePlayer())
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Update() across clubs error = %v, want core.ErrNotFound", err)
	}
}

func TestListReturnsOnlyTheCallersClub(t *testing.T) {
	svc, _ := newService()

	if _, err := svc.Add(context.Background(), "club-1", samplePlayer()); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if _, err := svc.Add(context.Background(), "club-2", samplePlayer()); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	list, err := svc.List(context.Background(), "club-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len(List()) = %d, want 1", len(list))
	}
}

func TestRemoveUnknownPlayerIsNotFound(t *testing.T) {
	svc, _ := newService()

	if err := svc.Remove(context.Background(), "club-1", "nope"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Remove() error = %v, want core.ErrNotFound", err)
	}
}
