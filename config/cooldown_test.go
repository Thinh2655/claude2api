package config

import (
	"testing"
	"time"
)

func TestSessionCooldown(t *testing.T) {
	key := "test-session-key"
	if SessionCooling(key) {
		t.Fatal("unknown session must not be cooling")
	}
	CooldownSession(key)
	if !SessionCooling(key) {
		t.Fatal("session must be cooling right after CooldownSession")
	}
	// Expired entries are dropped and reported as not cooling.
	cooldownMu.Lock()
	cooldownAt[key] = time.Now().Add(-time.Second)
	cooldownMu.Unlock()
	if SessionCooling(key) {
		t.Fatal("expired cooldown must report not cooling")
	}
	if _, ok := cooldownAt[key]; ok {
		t.Fatal("expired entry must be deleted")
	}
}