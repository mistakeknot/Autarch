package intermute

import (
	"context"

	shared "github.com/mistakeknot/autarch/internal/intermute"
)

func Start(ctx context.Context) (func(), error) {
	return shared.Start(ctx, shared.Options{
		Name:         "coldwine",
		Capabilities: []string{"coldwine"},
	})
}
