package qdrant

import (
	"context"
	"fmt"

	qdrantclient "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AdminOps is the subset of *qdrantclient.Client methods used during
// provisioning. Defined as an interface so tests can inject mocks without
// a live Qdrant server.
type AdminOps interface {
	CollectionExists(ctx context.Context, name string) (bool, error)
	GetCollectionInfo(ctx context.Context, name string) (*qdrantclient.CollectionInfo, error)
	CreateCollection(ctx context.Context, req *qdrantclient.CreateCollection) error
	CreateFieldIndex(ctx context.Context, req *qdrantclient.CreateFieldIndexCollection) (*qdrantclient.UpdateResult, error)
}

// EnsureCollection creates the named collection with the given vector size if
// it does not yet exist. If it already exists, the stored vector size is
// compared against wantSize — a mismatch is a hard error because silently
// storing wrong-dimension vectors would corrupt search results.
func EnsureCollection(ctx context.Context, ops AdminOps, collection string, wantSize int) error {
	exists, err := ops.CollectionExists(ctx, collection)
	if err != nil {
		return fmt.Errorf("check collection %q: %w", collection, err)
	}

	if exists {
		// Validate the stored vector size matches what we expect.
		info, err := ops.GetCollectionInfo(ctx, collection)
		if err != nil {
			return fmt.Errorf("get collection info for %q: %w", collection, err)
		}
		storedSize := collectionVectorSize(info)
		if storedSize != 0 && storedSize != uint64(wantSize) {
			return fmt.Errorf(
				"vector size mismatch for collection %q: stored=%d configured=%d — "+
					"run `docker compose down -v` to reset storage or update QDRANT_VECTOR_SIZE",
				collection, storedSize, wantSize,
			)
		}
		return nil
	}

	if err := ops.CreateCollection(ctx, &qdrantclient.CreateCollection{
		CollectionName: collection,
		VectorsConfig: qdrantclient.NewVectorsConfig(&qdrantclient.VectorParams{
			Size:     uint64(wantSize),
			Distance: qdrantclient.Distance_Cosine,
		}),
	}); err != nil {
		// AlreadyExists is safe to ignore — another process beat us to it.
		if st, ok := status.FromError(err); ok && st.Code() == codes.AlreadyExists {
			return nil
		}
		return fmt.Errorf("create collection %q: %w", collection, err)
	}

	return nil
}

// EnsureIndexes creates the payload field indexes required by the tool suite.
// Indexes are idempotent — calling this on an already-indexed collection is safe.
func EnsureIndexes(ctx context.Context, ops AdminOps, collection string) error {
	type fieldSpec struct {
		name      string
		fieldType qdrantclient.FieldType
	}

	fields := []fieldSpec{
		// new schema fields (memory_type replaces type)
		{"memory_type", qdrantclient.FieldType_FieldTypeKeyword},
		{"tags", qdrantclient.FieldType_FieldTypeKeyword},
		{"event_type", qdrantclient.FieldType_FieldTypeKeyword},
		{"status", qdrantclient.FieldType_FieldTypeKeyword},
		{"cache_key", qdrantclient.FieldType_FieldTypeKeyword},
		{"name", qdrantclient.FieldType_FieldTypeKeyword},
		{"ttl", qdrantclient.FieldType_FieldTypeDatetime},
		{"created", qdrantclient.FieldType_FieldTypeDatetime},
		{"updated", qdrantclient.FieldType_FieldTypeDatetime},
		{"content", qdrantclient.FieldType_FieldTypeText},
		// legacy fields kept for backward compatibility
		{"type", qdrantclient.FieldType_FieldTypeKeyword},
		{"session_id", qdrantclient.FieldType_FieldTypeKeyword},
	}

	for _, f := range fields {
		ft := f.fieldType
		if _, err := ops.CreateFieldIndex(ctx, &qdrantclient.CreateFieldIndexCollection{
			CollectionName: collection,
			FieldName:      f.name,
			FieldType:      &ft,
		}); err != nil {
			return fmt.Errorf("create index %q on %q: %w", f.name, collection, err)
		}
	}
	return nil
}

// collectionVectorSize extracts the configured vector size from CollectionInfo.
// Returns 0 if the information is not available (e.g. named vector config).
func collectionVectorSize(info *qdrantclient.CollectionInfo) uint64 {
	if info == nil {
		return 0
	}
	cfg := info.GetConfig()
	if cfg == nil {
		return 0
	}
	params := cfg.GetParams()
	if params == nil {
		return 0
	}
	vc := params.GetVectorsConfig()
	if vc == nil {
		return 0
	}
	if p := vc.GetParams(); p != nil {
		return p.GetSize()
	}
	return 0
}
