//go:build lancedb

package vectorstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/memory"

	"github.com/lancedb/lancedb-go/pkg/contracts"
	"github.com/lancedb/lancedb-go/pkg/lancedb"
)

const embeddingCol = "embedding"

// LanceStore implements Store using LanceDB at dataDir/vectors.lance.
type LanceStore struct {
	conn     contracts.IConnection
	dim      int
	alloc    memory.Allocator
	tableName string
}

// Open opens or creates a vector store at dataDir. The directory is created if missing.
// dimension is the embedding vector size (must match the provider).
func Open(ctx context.Context, dataDir string, dimension int) (Store, error) {
	if dimension <= 0 {
		return nil, fmt.Errorf("vectorstore: dimension must be positive")
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dataDir, "vectors.lance")
	conn, err := lancedb.Connect(ctx, dbPath, nil)
	if err != nil {
		return nil, fmt.Errorf("vectorstore: connect: %w", err)
	}
	s := &LanceStore{
		conn:      conn,
		dim:       dimension,
		alloc:     memory.NewGoAllocator(),
		tableName: tableName,
	}
	if err := s.ensureTable(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return s, nil
}

func (s *LanceStore) ensureTable(ctx context.Context) error {
	names, err := s.conn.TableNames(ctx)
	if err != nil {
		return err
	}
	for _, n := range names {
		if n == s.tableName {
			return nil
		}
	}
	schema, err := lancedb.NewSchemaBuilder().
		AddInt64Field("id", false).
		AddVectorField(embeddingCol, s.dim, contracts.VectorDataTypeFloat32, false).
		AddStringField("metadata", true).
		Build()
	if err != nil {
		return err
	}
	_, err = s.conn.CreateTable(ctx, s.tableName, schema)
	return err
}

func (s *LanceStore) table(ctx context.Context) (contracts.ITable, error) {
	return s.conn.OpenTable(ctx, s.tableName)
}

// Upsert inserts or replaces vectors. For each row id that already exists, existing rows are deleted then new rows are added.
func (s *LanceStore) Upsert(ctx context.Context, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	tbl, err := s.table(ctx)
	if err != nil {
		return err
	}
	defer tbl.Close()
	for _, r := range rows {
		if len(r.Vector) != s.dim {
			return fmt.Errorf("vectorstore: vector length %d != dimension %d", len(r.Vector), s.dim)
		}
	}
	// Remove existing rows with same ids so we replace
	idsToReplace := make(map[int64]struct{})
	for _, r := range rows {
		idsToReplace[r.ID] = struct{}{}
	}
	for id := range idsToReplace {
		_ = tbl.Delete(ctx, fmt.Sprintf("id = %d", id))
	}
	rec, err := s.rowsToRecord(rows)
	if err != nil {
		return err
	}
	defer rec.Release()
	return tbl.Add(ctx, rec, nil)
}

func (s *LanceStore) rowsToRecord(rows []Row) (arrow.Record, error) {
	arrowSchema := arrow.NewSchema(
		[]arrow.Field{
			{Name: "id", Type: arrow.PrimitiveTypes.Int64, Nullable: false},
			{Name: embeddingCol, Type: arrow.FixedSizeListOf(int32(s.dim), arrow.PrimitiveTypes.Float32), Nullable: false},
			{Name: "metadata", Type: arrow.BinaryTypes.String, Nullable: true},
		},
		nil,
	)
	idBuilder := array.NewInt64Builder(s.alloc)
	vecBuilder := array.NewFixedSizeListBuilder(s.alloc, int32(s.dim), arrow.PrimitiveTypes.Float32)
	metaBuilder := array.NewStringBuilder(s.alloc)
	defer idBuilder.Release()
	defer vecBuilder.Release()
	defer metaBuilder.Release()

	for _, r := range rows {
		idBuilder.Append(r.ID)
		vb := vecBuilder.ValueBuilder().(*array.Float32Builder)
		vb.AppendValues(r.Vector, nil)
		vecBuilder.Append(true)
		metaBuilder.Append(r.Metadata)
	}

	idArr := idBuilder.NewArray()
	defer idArr.Release()
	vecArr := vecBuilder.NewArray()
	defer vecArr.Release()
	metaArr := metaBuilder.NewArray()
	defer metaArr.Release()

	rec := array.NewRecord(arrowSchema, []arrow.Array{idArr, vecArr, metaArr}, int64(len(rows)))
	return rec, nil
}

// Search returns the k nearest chunk IDs for the query vector.
func (s *LanceStore) Search(ctx context.Context, query []float32, k int) ([]SearchResult, error) {
	if len(query) != s.dim {
		return nil, fmt.Errorf("vectorstore: query vector length %d != dimension %d", len(query), s.dim)
	}
	tbl, err := s.table(ctx)
	if err != nil {
		return nil, err
	}
	defer tbl.Close()
	rows, err := tbl.VectorSearch(ctx, embeddingCol, query, k)
	if err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(rows))
	for _, m := range rows {
		idVal, ok := m["id"]
		if !ok {
			continue
		}
		var id int64
		switch v := idVal.(type) {
		case int64:
			id = v
		case int32:
			id = int64(v)
		case int:
			id = int64(v)
		case float64:
			id = int64(v)
		default:
			continue
		}
		meta := ""
		if v, ok := m["metadata"]; ok && v != nil {
			if str, ok := v.(string); ok {
				meta = str
			}
		}
		out = append(out, SearchResult{ChunkID: id, Metadata: meta})
	}
	return out, nil
}

// DeleteChunk removes all vectors for the given chunk ID.
func (s *LanceStore) DeleteChunk(ctx context.Context, chunkID int64) error {
	tbl, err := s.table(ctx)
	if err != nil {
		return err
	}
	defer tbl.Close()
	return tbl.Delete(ctx, fmt.Sprintf("id = %d", chunkID))
}

// Close closes the database connection.
func (s *LanceStore) Close() error {
	return s.conn.Close()
}
