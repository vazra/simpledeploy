package docker

import (
	"context"
	"log"
	"strings"

	"github.com/docker/docker/api/types/network"
)

// EnsureNetwork inspects the named network; if not found, creates it as a bridge
// with Attachable: true. Idempotent.
func EnsureNetwork(ctx context.Context, c Client, name string) error {
	if _, err := c.NetworkInspect(ctx, name); err == nil {
		return nil
	} else if !isNotFound(err) {
		return err
	}
	if _, err := c.NetworkCreate(ctx, name, network.CreateOptions{
		Driver:     "bridge",
		Attachable: true,
	}); err != nil {
		return err
	}
	log.Printf("[docker] created shared network %q", name)
	return nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "No such network") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "404")
}
