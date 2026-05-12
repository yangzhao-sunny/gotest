package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

func NewClient(addr string) (*goredis.Client, error) {
	c := goredis.NewClient(&goredis.Options{Addr: addr})
	if err := c.Ping(context.Background()).Err(); err != nil {
		c.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return c, nil
}
