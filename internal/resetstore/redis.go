package resetstore

import (
	"context"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// Redis stores reset sessions as a hash per email (fields: code, attempts,
// token) with a CodeTTL expiry, plus a separate cooldown key for resend gating.
type Redis struct {
	client *redis.Client
}

// NewRedis parses a redis:// URL and returns a Store. Panics on a malformed URL
// (a misconfiguration that should fail fast at startup).
func NewRedis(url string) *Redis {
	opt, err := redis.ParseURL(url)
	if err != nil {
		panic("resetstore: invalid REDIS_URL: " + err.Error())
	}
	return &Redis{client: redis.NewClient(opt)}
}

func sessionKey(email string) string  { return "pwreset:sess:" + email }
func cooldownKey(email string) string { return "pwreset:cd:" + email }

// incrAttemptsScript increments the attempts field only when the session hash
// already exists, returning 0 otherwise. This mirrors Memory.IncrAttempts
// (no-op on a missing/expired session) and — crucially — avoids HINCRBY
// auto-creating a TTL-less orphan hash. The EXISTS+HINCRBY runs atomically in
// one Redis call, so there's no check-then-act race.
var incrAttemptsScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then
	return redis.call('HINCRBY', KEYS[1], 'attempts', 1)
else
	return 0
end
`)

func (r *Redis) MarkSent(ctx context.Context, email string) (bool, error) {
	// SET key 1 NX EX 60 — true only if we set it (no active cooldown).
	ok, err := r.client.SetNX(ctx, cooldownKey(email), "1", CooldownTTL).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (r *Redis) SetCode(ctx context.Context, email, codeHash string) error {
	key := sessionKey(email)
	pipe := r.client.TxPipeline()
	pipe.Del(ctx, key)
	pipe.HSet(ctx, key, "code", codeHash, "attempts", 0)
	pipe.Expire(ctx, key, CodeTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *Redis) Get(ctx context.Context, email string) (*Session, error) {
	vals, err := r.client.HGetAll(ctx, sessionKey(email)).Result()
	if err != nil {
		return nil, err
	}
	if len(vals) == 0 {
		return nil, nil
	}
	attempts, _ := strconv.Atoi(vals["attempts"])
	return &Session{
		CodeHash:  vals["code"],
		Attempts:  attempts,
		TokenHash: vals["token"],
	}, nil
}

func (r *Redis) IncrAttempts(ctx context.Context, email string) (int, error) {
	// Only increment when the session hash exists; never auto-create a
	// TTL-less orphan (see incrAttemptsScript).
	n, err := incrAttemptsScript.Run(ctx, r.client, []string{sessionKey(email)}).Int64()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (r *Redis) PromoteToToken(ctx context.Context, email, tokenHash string) error {
	key := sessionKey(email)
	pipe := r.client.TxPipeline()
	pipe.HDel(ctx, key, "code", "attempts")
	pipe.HSet(ctx, key, "token", tokenHash)
	pipe.Expire(ctx, key, CodeTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *Redis) Clear(ctx context.Context, email string) error {
	return r.client.Del(ctx, sessionKey(email), cooldownKey(email)).Err()
}
