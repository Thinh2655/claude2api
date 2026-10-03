package service

import (
	"claude2api/config"
	"testing"
)

func mkSession(key string) config.SessionInfo {
	return *config.NewSessionInfo(key, "")
}

func TestRoundRobinOrder(t *testing.T) {
	sessions := []config.SessionInfo{mkSession("a"), mkSession("b"), mkSession("c")}

	// rotation alone: start=1 gives [1,2,0]
	got := roundRobinOrder(sessions, 1)
	want := []int{1, 2, 0}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("start=1 expected %v got %v", want, got)
		}
	}

	// cooling account is pushed to the end
	config.CooldownSession("a")
	got = roundRobinOrder(sessions, 1)
	want = []int{1, 2, 0} // fresh: b,c ; cooling: a
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cooldown expected %v got %v", want, got)
		}
	}
	config.ClearCooldown()
}

func TestRoundRobinAllCooling(t *testing.T) {
	sessions := []config.SessionInfo{mkSession("a"), mkSession("b")}
	for k := range sessions {
		config.CooldownSession(sessions[k].SessionKey)
	}
	got := roundRobinOrder(sessions, 0)
	if len(got) != 2 {
		t.Fatalf("expected all sessions, got %v", got)
	}
	config.ClearCooldown()
}

func TestRoundRobinSkipsDisabled(t *testing.T) {
	sessions := []config.SessionInfo{mkSession("a"), mkSession("b"), mkSession("c")}
	sessions[1].Disabled = true // b off
	got := roundRobinOrder(sessions, 0)
	want := []int{0, 2}
	if len(got) != len(want) {
		t.Fatalf("expected %v got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v got %v", want, got)
		}
	}
}

func TestRoundRobinAllDisabled(t *testing.T) {
	sessions := []config.SessionInfo{mkSession("a"), mkSession("b")}
	sessions[0].Disabled = true
	sessions[1].Disabled = true
	if got := roundRobinOrder(sessions, 0); len(got) != 0 {
		t.Fatalf("expected empty list, got %v", got)
	}
}