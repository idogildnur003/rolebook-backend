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
	n, err := r.client.HIncrBy(ctx, sessionKey(email), "attempts", 1).Result()
	return int(n), err
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
