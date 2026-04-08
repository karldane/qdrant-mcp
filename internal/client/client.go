package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"

	"github.com/karldane/qdrant-mcp/internal/config"
	qdrant "github.com/qdrant/go-client/qdrant"
)

type Client struct {
	adminClient *qdrant.Client
	userClient  *qdrant.Client
	collection  string
	cfg         *config.Config
}

func New(cfg *config.Config) (*Client, error) {
	c := &Client{
		cfg:        cfg,
		collection: cfg.Collection,
	}

	if cfg.AdminURL == "" {
		return nil, fmt.Errorf("QDRANT_ADMIN_URL is required")
	}

	host, port, useTLS, err := parseURL(cfg.AdminURL)
	if err != nil {
		return nil, fmt.Errorf("parse QDRANT_ADMIN_URL: %w", err)
	}

	adminClient, err := qdrant.NewClient(&qdrant.Config{
		Host:                   host,
		Port:                   port,
		APIKey:                 cfg.AdminKey,
		UseTLS:                 useTLS,
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create admin client: %w", err)
	}
	c.adminClient = adminClient

	userAPIKey := deriveUserAPIKey(cfg.Username, cfg.UserSecret)
	userClient, err := qdrant.NewClient(&qdrant.Config{
		Host:                   host,
		Port:                   port,
		APIKey:                 userAPIKey,
		UseTLS:                 useTLS,
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create user client: %w", err)
	}
	c.userClient = userClient

	return c, nil
}

// parseURL extracts host, port, and TLS flag from a URL string.
// Accepts e.g. "http://localhost:6334" or "https://example.com:6334".
// Falls back to port 6334 if not specified.
func parseURL(rawURL string) (host string, port int, useTLS bool, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", 0, false, err
	}
	host = u.Hostname()
	if host == "" {
		host = rawURL // treat as bare host
	}
	useTLS = u.Scheme == "https"
	portStr := u.Port()
	if portStr == "" {
		port = 6334
	} else {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return "", 0, false, fmt.Errorf("invalid port %q: %w", portStr, err)
		}
	}
	return host, port, useTLS, nil
}

func deriveUserAPIKey(username, secret string) string {
	if username == "" || secret == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(username + secret))
	return hex.EncodeToString(hash[:])
}

func (c *Client) EnsureCollection(ctx context.Context) error {
	if c.adminClient == nil {
		return fmt.Errorf("admin client not initialized")
	}

	exists, err := c.adminClient.CollectionExists(ctx, c.collection)
	if err != nil {
		return fmt.Errorf("check collection: %w", err)
	}
	if exists {
		return nil
	}

	err = c.adminClient.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: c.collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(c.cfg.VectorSize),
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	return nil
}

func (c *Client) UpsertPoint(ctx context.Context, id string, vector []float64, payload map[string]interface{}) error {
	valueMap, err := qdrant.TryValueMap(payload)
	if err != nil {
		return fmt.Errorf("convert payload: %w", err)
	}

	point := &qdrant.PointStruct{
		Id:      &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: id}},
		Payload: valueMap,
	}

	if len(vector) > 0 {
		f32 := make([]float32, len(vector))
		for i, v := range vector {
			f32[i] = float32(v)
		}
		point.Vectors = qdrant.NewVectors(f32...)
	}

	_, err = c.userClient.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: c.collection,
		Points:         []*qdrant.PointStruct{point},
	})
	return err
}

func (c *Client) Search(ctx context.Context, query []float64, limit int, filter map[string]interface{}) ([]SearchResult, error) {
	f32 := make([]float32, len(query))
	for i, v := range query {
		f32[i] = float32(v)
	}

	req := &qdrant.QueryPoints{
		CollectionName: c.collection,
		Query:          qdrant.NewQuery(f32...),
		Limit:          qdrant.PtrOf(uint64(limit)),
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
	}

	if len(filter) > 0 {
		req.Filter = &qdrant.Filter{Must: filterToQdrant(filter)}
	}

	scored, err := c.userClient.Query(ctx, req)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(scored))
	for _, r := range scored {
		results = append(results, SearchResult{
			ID:      pointIDToString(r.GetId()),
			Score:   r.GetScore(),
			Payload: valueMapToInterface(r.GetPayload()),
		})
	}
	return results, nil
}

func (c *Client) Scroll(ctx context.Context, limit int, filter map[string]interface{}, offset string) ([]ScrollResult, string, error) {
	lim := uint32(limit)
	req := &qdrant.ScrollPoints{
		CollectionName: c.collection,
		Limit:          &lim,
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
	}

	if len(filter) > 0 {
		req.Filter = &qdrant.Filter{Must: filterToQdrant(filter)}
	}

	if offset != "" {
		req.Offset = &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: offset}}
	}

	// Use low-level client to get NextPageOffset from ScrollResponse.
	resp, err := c.userClient.GetPointsClient().Scroll(ctx, req)
	if err != nil {
		return nil, "", err
	}

	results := make([]ScrollResult, 0, len(resp.GetResult()))
	for _, r := range resp.GetResult() {
		results = append(results, ScrollResult{
			ID:      pointIDToString(r.GetId()),
			Payload: valueMapToInterface(r.GetPayload()),
		})
	}

	var nextOffset string
	if npo := resp.GetNextPageOffset(); npo != nil {
		nextOffset = pointIDToString(npo)
	}

	return results, nextOffset, nil
}

func (c *Client) GetPoint(ctx context.Context, id string) (*GetResult, error) {
	results, err := c.userClient.Get(ctx, &qdrant.GetPoints{
		CollectionName: c.collection,
		Ids:            []*qdrant.PointId{{PointIdOptions: &qdrant.PointId_Uuid{Uuid: id}}},
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		WithVectors:    &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: true}},
	})
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("point not found: %s", id)
	}

	r := results[0]
	var vec []float32
	if v := r.GetVectors(); v != nil {
		if dv := v.GetVector(); dv != nil {
			vec = dv.GetDense().GetData()
		}
	}

	return &GetResult{
		ID:      pointIDToString(r.GetId()),
		Vector:  vec,
		Payload: valueMapToInterface(r.GetPayload()),
	}, nil
}

func (c *Client) DeletePoints(ctx context.Context, ids []string, filter map[string]interface{}) error {
	if len(ids) > 0 {
		pointIDs := make([]*qdrant.PointId, 0, len(ids))
		for _, id := range ids {
			pointIDs = append(pointIDs, &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: id}})
		}
		_, err := c.userClient.Delete(ctx, &qdrant.DeletePoints{
			CollectionName: c.collection,
			Points:         qdrant.NewPointsSelector(pointIDs...),
		})
		return err
	}

	_, err := c.userClient.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: c.collection,
		Points:         qdrant.NewPointsSelectorFilter(&qdrant.Filter{Must: filterToQdrant(filter)}),
	})
	return err
}

// Result types

type SearchResult struct {
	ID      string
	Score   float32
	Payload map[string]interface{}
}

type ScrollResult struct {
	ID      string
	Payload map[string]interface{}
}

type GetResult struct {
	ID      string
	Vector  []float32
	Payload map[string]interface{}
}

// filterToQdrant converts a simple key=value map into Qdrant filter conditions.
func filterToQdrant(filter map[string]interface{}) []*qdrant.Condition {
	if filter == nil {
		return nil
	}
	conds := make([]*qdrant.Condition, 0, len(filter))
	for k, v := range filter {
		conds = append(conds, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: k,
					Match: &qdrant.Match{
						MatchValue: &qdrant.Match_Keyword{
							Keyword: fmt.Sprintf("%v", v),
						},
					},
				},
			},
		})
	}
	return conds
}

// valueMapToInterface converts a Qdrant Value map back to map[string]interface{}.
func valueMapToInterface(payload map[string]*qdrant.Value) map[string]interface{} {
	if payload == nil {
		return nil
	}
	m := make(map[string]interface{}, len(payload))
	for k, v := range payload {
		m[k] = valueToInterface(v)
	}
	return m
}

func valueToInterface(v *qdrant.Value) interface{} {
	if v == nil {
		return nil
	}
	switch x := v.Kind.(type) {
	case *qdrant.Value_NullValue:
		return nil
	case *qdrant.Value_BoolValue:
		return x.BoolValue
	case *qdrant.Value_IntegerValue:
		return x.IntegerValue
	case *qdrant.Value_DoubleValue:
		return x.DoubleValue
	case *qdrant.Value_StringValue:
		return x.StringValue
	case *qdrant.Value_StructValue:
		if x.StructValue == nil {
			return nil
		}
		return valueMapToInterface(x.StructValue.Fields)
	case *qdrant.Value_ListValue:
		if x.ListValue == nil {
			return nil
		}
		list := make([]interface{}, len(x.ListValue.Values))
		for i, lv := range x.ListValue.Values {
			list[i] = valueToInterface(lv)
		}
		return list
	default:
		return nil
	}
}

// pointIDToString converts a *qdrant.PointId to its string representation.
func pointIDToString(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	switch x := id.PointIdOptions.(type) {
	case *qdrant.PointId_Uuid:
		return x.Uuid
	case *qdrant.PointId_Num:
		return strconv.FormatUint(x.Num, 10)
	default:
		return ""
	}
}
