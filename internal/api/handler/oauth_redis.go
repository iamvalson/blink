package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisOAuthTransactions struct{ redis *redis.Client }

func NewRedisOAuthTransactions(client *redis.Client) *RedisOAuthTransactions {
	return &RedisOAuthTransactions{redis: client}
}

func (s *RedisOAuthTransactions) Create(tx OAuthTransaction) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	body, err := json.Marshal(tx)
	if err != nil {
		return err
	}
	return s.redis.Set(ctx, "blink:oauth:"+tx.Platform+":"+tx.State, body, time.Until(tx.ExpiresAt)).Err()
}

func (s *RedisOAuthTransactions) ForPlatform(platform string) OAuthTransactionStore {
	return platformOAuthTransactions{RedisOAuthTransactions: s, platform: platform}
}

type platformOAuthTransactions struct {
	*RedisOAuthTransactions
	platform string
}

func (s platformOAuthTransactions) Consume(state string) (OAuthTransaction, bool) {
	var tx OAuthTransaction
	if len(state) != 44 {
		return tx, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	data, err := s.redis.GetDel(ctx, "blink:oauth:"+s.platform+":"+state).Bytes()
	if err != nil || json.Unmarshal(data, &tx) != nil || !tx.ExpiresAt.After(time.Now()) {
		return OAuthTransaction{}, false
	}
	return tx, true
}
