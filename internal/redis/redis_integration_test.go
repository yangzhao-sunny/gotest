//go:build integration

package redis

import (
	"context"
	"testing"

	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func TestNewClient_Integration(t *testing.T) {
	ctx := context.Background()
	rc, err := tcredis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rc.Terminate(ctx) })

	addr, err := rc.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// strip redis:// prefix that testcontainers returns
	addr = stripScheme(addr)

	client, err := NewClient(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
}

func stripScheme(addr string) string {
	for _, prefix := range []string{"redis://", "rediss://"} {
		if len(addr) > len(prefix) && addr[:len(prefix)] == prefix {
			return addr[len(prefix):]
		}
	}
	return addr
}
