package core

import (
	"claude2api/config"
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAccountCompletionReservation(t *testing.T) {
	const first = "fixture-first-session"
	const alias = "fixture-another-session-for-first-account"
	const second = "fixture-second-account"
	ids := map[string]string{first: "first", alias: "first", second: "second"}
	completionAccounts.Lock()
	for key, id := range ids {
		completionAccounts.credentials[sha256.Sum256([]byte(key))] = accountIdentity{
			id: sha256.Sum256([]byte(id)), expires: time.Now().Add(time.Minute),
		}
	}
	completionAccounts.Unlock()
	t.Cleanup(func() {
		completionAccounts.Lock()
		defer completionAccounts.Unlock()
		for key := range ids {
			delete(completionAccounts.credentials, sha256.Sum256([]byte(key)))
		}
	})
	acquire := func(key string) func() {
		t.Helper()
		client := &Client{SessionKey: key}
		release, err := client.TryStartCompletion(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return release
	}
	releaseFirst := acquire(first)
	defer func() {
		if releaseFirst != nil {
			releaseFirst()
		}
	}()
	for _, key := range []string{first, alias} {
		client := &Client{SessionKey: key}
		if release, err := client.TryStartCompletion(context.Background()); !errors.Is(err, ErrAccountBusy) {
			if release != nil {
				release()
			}
			t.Fatalf("another request for the same account got %v, want ErrAccountBusy", err)
		}
	}
	defer acquire(second)()

	// The browser gateway uses the same reservation before proxying.
	previous := config.ConfigInstance
	config.ConfigInstance = &config.Config{Sessions: []config.SessionInfo{{SessionKey: alias}}}
	t.Cleanup(func() { config.ConfigInstance = previous })
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/organizations/org/chat_conversations/chat/completion", nil)
	(&gatewayHandler{}).ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Retry-After") == "" {
		t.Fatalf("busy gateway returned %d with headers %v", response.Code, response.Header())
	}

	releaseFirst()
	releaseFirst = nil
	acquire(alias)()
}
