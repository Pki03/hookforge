package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	*redis.Client
}

func (c *Client) Raw() *redis.Client {
	return c.Client
}

func Connect(redisURL string) (*Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis url: %w", err)
	}

	rdb := redis.NewClient(opts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	return &Client{rdb}, nil
}

func (c *Client) EnqueueEvent(ctx context.Context, eventID string) error {
	return c.LPush(ctx, "events:queue", eventID).Err()
}

func (c *Client) DequeueEvent(ctx context.Context) (string, error) {
	result, err := c.BRPop(ctx, 0, "events:queue").Result()
	if err != nil {
		return "", err
	}
	return result[1], nil
}
