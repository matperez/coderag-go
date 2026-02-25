//go:build !lancedb

package vectorstore

import (
	"context"
	"fmt"
)

// Open returns (nil, error) when built without the lancedb tag.
// Build with -tags lancedb for vector store support.
func Open(ctx context.Context, dataDir string, dimension int) (Store, error) {
	return nil, fmt.Errorf("vector store requires build with -tags lancedb")
}
