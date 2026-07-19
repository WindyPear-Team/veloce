package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

const (
	billingBalanceTTL = 10 * time.Minute
	billingLockTTL    = 30 * time.Second
	billingLockWait   = 5 * time.Second
)

var (
	billingLockRelease = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
end
return 0
`)
	billingLockRenew = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("pexpire", KEYS[1], ARGV[2])
end
return 0
`)
)

// AcquireUserBillingLock serializes billing transactions for a user across
// application instances. Redis is only a coordinator: any cache error falls
// back to the database's existing atomic balance update.
func AcquireUserBillingLock(ctx context.Context, userID uint) func() {
	client := Client()
	if client == nil || userID == 0 {
		return func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	token, err := newBillingLockToken()
	if err != nil {
		log.Printf("Redis billing lock token failed for user %d: %v; using database charge", userID, err)
		return func() {}
	}
	key := billingLockKey(userID)
	deadline := time.Now().Add(billingLockWait)
	for {
		acquired, err := client.SetNX(ctx, key, token, billingLockTTL).Result()
		if err != nil {
			log.Printf("Redis billing lock failed for user %d: %v; using database charge", userID, err)
			return func() {}
		}
		if acquired {
			return maintainBillingLock(client, key, token)
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			log.Printf("Redis billing lock timed out for user %d; using database charge", userID)
			return func() {}
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return func() {}
		case <-timer.C:
		}
	}
}

// StoreUserBillingBalance writes a committed user balance to Redis. The value
// is informational and is never treated as the billing source of truth.
func StoreUserBillingBalance(ctx context.Context, userID uint, balance decimal.Decimal) {
	client := Client()
	if client == nil || userID == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := client.Set(ctx, billingBalanceKey(userID), balance.String(), billingBalanceTTL).Err(); err != nil {
		log.Printf("Redis billing balance update failed for user %d: %v", userID, err)
	}
}

// InvalidateUserBillingBalance discards a balance mirror after a rolled back
// transaction or a balance update that cannot provide a committed value.
func InvalidateUserBillingBalance(ctx context.Context, userID uint) {
	client := Client()
	if client == nil || userID == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := client.Del(ctx, billingBalanceKey(userID)).Err(); err != nil {
		log.Printf("Redis billing balance invalidation failed for user %d: %v", userID, err)
	}
}

func maintainBillingLock(client *redis.Client, key, token string) func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(billingLockTTL / 3)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := billingLockRenew.Run(context.Background(), client, []string{key}, token, billingLockTTL.Milliseconds()).Err(); err != nil {
					log.Printf("Redis billing lock renewal failed: %v", err)
				}
			}
		}
	}()
	return func() {
		close(done)
		if err := billingLockRelease.Run(context.Background(), client, []string{key}, token).Err(); err != nil {
			log.Printf("Redis billing lock release failed: %v", err)
		}
	}
}

func newBillingLockToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func billingLockKey(userID uint) string {
	return fmt.Sprintf("veloce:billing:user:%d:lock", userID)
}

func billingBalanceKey(userID uint) string {
	return fmt.Sprintf("veloce:billing:user:%d:balance", userID)
}
