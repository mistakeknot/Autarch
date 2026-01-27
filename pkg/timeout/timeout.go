// Package timeout provides named timeout constants for Autarch tools.
package timeout

import "time"

const (
	// HTTPDefault is the default timeout for HTTP requests.
	HTTPDefault = 10 * time.Second

	// DBWrite is the timeout for database write operations.
	DBWrite = 5 * time.Second

	// Shutdown is the timeout for graceful shutdown sequences.
	Shutdown = 5 * time.Second
)
