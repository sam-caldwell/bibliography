package main

import (
	"context"
	"os"

	"bibliography/src/cmd/bib/addcmd"
)

// getEnv returns the environment value for key or def if unset.
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// doAdd is a convenience wrapper used in tests; delegates to addcmd implementation.
func doAdd(ctx context.Context, typ string, hints map[string]string) error {
	return addcmd.AddWithKeywords(ctx, commitAndPush, typ, hints, nil)
}
