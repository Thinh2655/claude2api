package core

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"time"
)

var ErrAccountBusy = errors.New("Claude account already has a request in progress")

type accountIdentity struct {
	id      [32]byte
	expires time.Time
}

var completionAccounts = struct {
	sync.Mutex
	credentials map[[32]byte]accountIdentity
	active      map[[32]byte]bool
}{credentials: make(map[[32]byte]accountIdentity), active: make(map[[32]byte]bool)}

// TryStartCompletion reserves the verified account across API entry points,
// including when different credentials resolve to the same Claude account.
func (c *Client) TryStartCompletion(ctx context.Context) (func(), error) {
	key := sha256.Sum256([]byte(c.SessionKey))
	completionAccounts.Lock()
	identity := completionAccounts.credentials[key]
	completionAccounts.Unlock()
	if time.Now().After(identity.expires) {
		verifyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		email, err := c.getAccountEmail(verifyCtx)
		if err != nil {
			return nil, err
		}
		identity = accountIdentity{
			id:      sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email)))),
			expires: time.Now().Add(10 * time.Minute),
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	completionAccounts.Lock()
	defer completionAccounts.Unlock()
	if len(completionAccounts.credentials) >= 1024 {
		clear(completionAccounts.credentials)
	}
	completionAccounts.credentials[key] = identity
	if completionAccounts.active[identity.id] {
		return nil, ErrAccountBusy
	}
	completionAccounts.active[identity.id] = true
	return func() {
		completionAccounts.Lock()
		delete(completionAccounts.active, identity.id)
		completionAccounts.Unlock()
	}, nil
}
