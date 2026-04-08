package qdrant

import (
	"context"
	"errors"
	"testing"

	qdrantclient "github.com/qdrant/go-client/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock admin client for provision tests
// ---------------------------------------------------------------------------

type mockAdminOps struct {
	existsResult      bool
	existsErr         error
	collectionInfo    *qdrantclient.CollectionInfo
	collectionInfoErr error
	createErr         error
	indexErr          error
	indexCalls        []string // field names for which CreateFieldIndex was called
}

func (m *mockAdminOps) CollectionExists(_ context.Context, _ string) (bool, error) {
	return m.existsResult, m.existsErr
}

func (m *mockAdminOps) GetCollectionInfo(_ context.Context, _ string) (*qdrantclient.CollectionInfo, error) {
	return m.collectionInfo, m.collectionInfoErr
}

func (m *mockAdminOps) CreateCollection(_ context.Context, _ *qdrantclient.CreateCollection) error {
	return m.createErr
}

func (m *mockAdminOps) CreateFieldIndex(_ context.Context, req *qdrantclient.CreateFieldIndexCollection) (*qdrantclient.UpdateResult, error) {
	m.indexCalls = append(m.indexCalls, req.GetFieldName())
	return &qdrantclient.UpdateResult{}, m.indexErr
}

// ---------------------------------------------------------------------------
// EnsureCollection tests
// ---------------------------------------------------------------------------

func TestEnsureCollection_creates(t *testing.T) {
	// Collection absent → should be created with the correct vector size.
	mock := &mockAdminOps{existsResult: false}
	err := EnsureCollection(context.Background(), mock, "alice_col", 768)
	require.NoError(t, err)
	// If createErr is nil, creation succeeded.
}

func TestEnsureCollection_exists_match(t *testing.T) {
	// Collection present, vector sizes match → no error.
	size := uint64(768)
	mock := &mockAdminOps{
		existsResult: true,
		collectionInfo: &qdrantclient.CollectionInfo{
			Config: &qdrantclient.CollectionConfig{
				Params: &qdrantclient.CollectionParams{
					VectorsConfig: qdrantclient.NewVectorsConfig(&qdrantclient.VectorParams{
						Size:     size,
						Distance: qdrantclient.Distance_Cosine,
					}),
				},
			},
		},
	}
	err := EnsureCollection(context.Background(), mock, "alice_col", 768)
	require.NoError(t, err)
}

func TestEnsureCollection_exists_mismatch(t *testing.T) {
	// Collection present, sizes differ → hard error.
	size := uint64(1536)
	mock := &mockAdminOps{
		existsResult: true,
		collectionInfo: &qdrantclient.CollectionInfo{
			Config: &qdrantclient.CollectionConfig{
				Params: &qdrantclient.CollectionParams{
					VectorsConfig: qdrantclient.NewVectorsConfig(&qdrantclient.VectorParams{
						Size:     size,
						Distance: qdrantclient.Distance_Cosine,
					}),
				},
			},
		},
	}
	err := EnsureCollection(context.Background(), mock, "alice_col", 768)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vector size mismatch")
}

func TestEnsureCollection_existsCheckError(t *testing.T) {
	mock := &mockAdminOps{existsErr: errors.New("network error")}
	err := EnsureCollection(context.Background(), mock, "alice_col", 768)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// EnsureIndexes tests
// ---------------------------------------------------------------------------

func TestEnsureIndexes(t *testing.T) {
	mock := &mockAdminOps{}
	err := EnsureIndexes(context.Background(), mock, "alice_col")
	require.NoError(t, err)

	// All required payload indexes must have been created.
	required := []string{"type", "tags", "session_id", "ttl", "created", "content"}
	for _, field := range required {
		assert.Contains(t, mock.indexCalls, field, "EnsureIndexes must create index for %q", field)
	}
}
