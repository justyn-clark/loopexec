package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/justyn-clark/loopexec/internal/workflow"
)

func scopedFixture(t *testing.T, max int, phases map[string]int) (string, scopedBudgetPolicy) {
	t.Helper()
	store := t.TempDir()
	p := scopedBudgetPolicy{SchemaVersion: 1, Scope: "studio-floor-rack", MaxCalls: max, PhaseLimits: phases}
	file := filepath.Join(store, "policy.json")
	if err := workflow.Atomic(file, p); err != nil {
		t.Fatal(err)
	}
	read, err := readScopedPolicy(file)
	if err != nil || workflow.JSONHash(read) != workflow.JSONHash(p) {
		t.Fatalf("policy read: %v", err)
	}
	return store, p
}

func TestScopedBudgetAcrossRunIDsAndPhases(t *testing.T) {
	store, p := scopedFixture(t, 2, map[string]int{"builder": 1, "critic": 1, "critic-camera": 0})
	if _, err := reserveScopedCall(store, p, "run-a/critic-camera", "critic-camera"); !errors.Is(err, errScopedExhausted) {
		t.Fatalf("camera allowance: %v", err)
	}
	first, err := reserveScopedCall(store, p, "run-a/builder", "builder")
	if err != nil || first.Used != 1 || first.Remaining != 1 {
		t.Fatalf("first reservation: %+v %v", first, err)
	}
	if _, err := reserveScopedCall(store, p, "run-b/builder", "builder"); !errors.Is(err, errScopedExhausted) {
		t.Fatalf("run-id reset builder allowance: %v", err)
	}
	second, err := reserveScopedCall(store, p, "run-b/critic", "critic")
	if err != nil || second.Used != 2 || second.Remaining != 0 {
		t.Fatalf("second reservation: %+v %v", second, err)
	}
	if _, err := reserveScopedCall(store, p, "run-c/critic", "critic"); !errors.Is(err, errScopedExhausted) {
		t.Fatalf("run-id reset total allowance: %v", err)
	}
	if _, err := reserveScopedCall(store, p, "run-b/critic", "critic"); !errors.Is(err, errScopedDuplicate) {
		t.Fatalf("duplicate call could be retried: %v", err)
	}
}

func TestScopedBudgetConcurrentReservation(t *testing.T) {
	store, p := scopedFixture(t, 1, map[string]int{"critic": 1})
	var group sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			_, err := reserveScopedCall(store, p, "run-"+string(rune('a'+i)), "critic")
			results <- err
		}(i)
	}
	group.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("%d concurrent calls reserved one slot", success)
	}
	status, _, err := scopedStatus(filepath.Join(store, p.Scope), p)
	if err != nil || status.Used != 1 {
		t.Fatalf("ledger after concurrent calls: %+v %v", status, err)
	}
}

func TestScopedBudgetPinsPolicyAndFailsClosedOnDamage(t *testing.T) {
	store, p := scopedFixture(t, 2, map[string]int{"builder": 1, "critic": 1})
	if _, err := reserveScopedCall(store, p, "run-a/builder", "builder"); err != nil {
		t.Fatal(err)
	}
	changed := p
	changed.MaxCalls = 3
	if _, err := reserveScopedCall(store, changed, "run-b/critic", "critic"); !errors.Is(err, errScopedPolicy) {
		t.Fatalf("policy raise changed live cap: %v", err)
	}
	file := filepath.Join(store, p.Scope, "calls", "call-000002.json")
	if err := os.WriteFile(file, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reserveScopedCall(store, p, "run-b/critic", "critic"); !errors.Is(err, errScopedPolicy) {
		t.Fatalf("damaged ledger was ignored: %v", err)
	}
}

func TestScopedBudgetZeroCap(t *testing.T) {
	store, p := scopedFixture(t, 0, map[string]int{"critic": 0})
	if _, err := reserveScopedCall(store, p, "new-run/critic", "critic"); !errors.Is(err, errScopedExhausted) {
		t.Fatalf("zero cap: %v", err)
	}
}
