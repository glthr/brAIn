package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/glthr/brAIn/internal/model"
)

// MemoryEmbedding holds a memory ID paired with its embedding vector.
type MemoryEmbedding struct {
	ID  int64
	Vec []float32
}

// UpsertEmbedding stores (or replaces) the embedding vector for a memory.
func (s *Store) UpsertEmbedding(ctx context.Context, memID int64, vec []float32) error {
	blob, err := float32SliceToBlob(vec)
	if err != nil {
		return fmt.Errorf("store embedding id=%d: %w", memID, err)
	}
	_, err = s.DB.ExecContext(ctx,
		`UPDATE memories SET embedding = ? WHERE id = ?`, blob, memID)
	return err
}

// SemanticEmbeddings returns all semantic (and episodic) memories that have an embedding.
// Used by hybrid Lookup to perform in-process cosine similarity ranking.
func (s *Store) SemanticEmbeddings(ctx context.Context) ([]MemoryEmbedding, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, embedding FROM memories
		WHERE memory_type IN ('semantic', 'episodic')
		  AND embedding IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []MemoryEmbedding
	for rows.Next() {
		var id int64
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, err
		}
		vec, err := blobToFloat32Slice(blob)
		if err != nil {
			continue // skip corrupted blobs
		}
		result = append(result, MemoryEmbedding{ID: id, Vec: vec})
	}
	return result, rows.Err()
}

// MemoriesWithoutEmbedding returns long-term memories (episodic + semantic) that
// have not yet been embedded, up to limit records.
func (s *Store) MemoriesWithoutEmbedding(ctx context.Context, limit int) ([]model.Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.QueryMemories(ctx, `
		SELECT `+MemoryCols+`
		FROM memories
		WHERE memory_type IN ('episodic', 'semantic')
		  AND embedding IS NULL
		ORDER BY created_at DESC
		LIMIT ?`, limit)
}

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// Returns 0 if either vector is zero-length or dimensions don't match.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// float32SliceToBlob encodes a []float32 as a little-endian byte slice.
func float32SliceToBlob(vec []float32) ([]byte, error) {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		bits := math.Float32bits(v)
		binary.LittleEndian.PutUint32(buf[i*4:], bits)
	}
	return buf, nil
}

// blobToFloat32Slice decodes a little-endian byte slice into a []float32.
func blobToFloat32Slice(blob []byte) ([]float32, error) {
	if len(blob)%4 != 0 {
		return nil, fmt.Errorf("blob length %d not divisible by 4", len(blob))
	}
	vec := make([]float32, len(blob)/4)
	for i := range vec {
		bits := binary.LittleEndian.Uint32(blob[i*4:])
		vec[i] = math.Float32frombits(bits)
	}
	return vec, nil
}

// EmbeddingFromRow scans an embedding BLOB from a nullable column.
// Returns nil (no error) when the column is NULL.
func EmbeddingFromRow(blob sql.NullString) ([]float32, error) {
	if !blob.Valid || blob.String == "" {
		return nil, nil
	}
	return blobToFloat32Slice([]byte(blob.String))
}
