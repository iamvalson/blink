package jobs

import (
	"fmt"
	"strings"

	"github.com/hibiken/asynq"
)

type Client struct {
	client *asynq.Client
}

func NewClient(redisAddr string) (*Client, error) {
	// Strip redis:// scheme if present (asynq expects host:port format)
	redisAddr = strings.TrimPrefix(redisAddr, "redis://")
	redisAddr = strings.TrimPrefix(redisAddr, "redis+srv://")

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})

	// Test connection
	if err := client.Ping(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &Client{client: client}, nil
}


func (c *Client) Enqueue(task *asynq.Task, opts ...asynq.Option) (string, error) {
	info, err := c.client.Enqueue(task, opts...)
	if err != nil{
		return "", err
	}
	return info.ID, nil
}


func (c *Client) Close() error {
	return c.client.Close()
}