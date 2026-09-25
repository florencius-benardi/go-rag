package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"go-rag/internal/domain/models"
	"gorm.io/gorm"
)

type QueryEmbedder interface {
	EmbedQuery(context.Context, string) ([]float32, error)
}

type Store struct {
	db             *gorm.DB
	embedder       QueryEmbedder
	embeddingModel string
	historyLimit   int
	// merchantID selects the outlet whose merchant_catalogs overrides apply.
	// 0 means no outlet is configured: no merchant_catalogs row has that id,
	// so Facts falls back to the catalog's own price and stock unchanged.
	merchantID int32
}

func NewStore(db *gorm.DB, embedder QueryEmbedder, embeddingModel string, merchantID int32) *Store {
	return &Store{db: db, embedder: embedder, embeddingModel: embeddingModel, historyLimit: 5, merchantID: merchantID}
}

// NewStoreWithAllHistory loads every persisted turn for a CLI chat session.
func NewStoreWithAllHistory(db *gorm.DB, embedder QueryEmbedder, embeddingModel string, merchantID int32) *Store {
	return &Store{db: db, embedder: embedder, embeddingModel: embeddingModel, merchantID: merchantID}
}

func (s *Store) Recent(ctx context.Context, conversationID string) ([]Message, error) {
	var turns []models.RagTurn
	query := s.db.WithContext(ctx).Select("id", "user_message", "answer").
		Where("conversation_id = ?", conversationID).Order("id DESC")
	if s.historyLimit > 0 {
		query = query.Limit(s.historyLimit)
	}
	if err := query.Find(&turns).Error; err != nil {
		return nil, err
	}
	messages := make([]Message, 0, len(turns)*2)
	for i := len(turns) - 1; i >= 0; i-- {
		messages = append(messages, Message{"user", turns[i].UserMessage}, Message{"assistant", turns[i].Answer})
	}
	return messages, nil
}

func (s *Store) Resolve(ctx context.Context, names []string) ([]Entity, bool, error) {
	var entities []Entity
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var rows []models.Catalogs
		if err := s.db.WithContext(ctx).Where("is_active = 1 AND LOWER(title) = LOWER(?)", name).
			Limit(3).Find(&rows).Error; err != nil {
			return nil, false, err
		}
		if len(rows) == 0 {
			if err := s.db.WithContext(ctx).Where("is_active = 1 AND title ILIKE ?", "%"+escapeLike(name)+"%").
				Limit(3).Find(&rows).Error; err != nil {
				return nil, false, err
			}
		}
		if len(rows) > 1 {
			return entities, true, nil
		}
		if len(rows) == 1 {
			entities = append(entities, Entity{CatalogID: rows[0].ID, Title: textValue(rows[0].Title)})
		}
	}
	return entities, false, nil
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

func textValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Store) Facts(ctx context.Context, entities []Entity) ([]Fact, error) {
	if len(entities) == 0 {
		return nil, nil
	}
	ids := make([]int32, 0, len(entities))
	for _, entity := range entities {
		ids = append(ids, entity.CatalogID)
	}
	// The merchant join is unconditional: with merchantID 0 (no outlet
	// configured) or a product this outlet never overrode, nothing matches and
	// price/stock come from the catalog exactly as before this existed.
	var productRows []struct {
		ID    int32
		Title *string
		SKU   string
		Price *int32
		Stock int32
		Promo bool
	}
	const factsQuery = `SELECT p.id, p.title, p.sku,
		COALESCE(m.price, p.price) AS price,
		CASE WHEN m.id IS NOT NULL AND COALESCE(m.is_available, 1) = 0 THEN 0 ELSE p.stock END AS stock,
		(m.id IS NOT NULL AND m.price IS DISTINCT FROM p.price) AS promo
		FROM catalogs p
		LEFT JOIN merchant_catalogs m ON m.catalog_id = p.id AND m.merchant_id = ? AND m.is_active = 1
		WHERE p.id IN ? AND p.is_active = 1`
	if err := s.db.WithContext(ctx).Raw(factsQuery, s.merchantID, ids).Scan(&productRows).Error; err != nil {
		return nil, fmt.Errorf("load facts with merchant pricing: %w", err)
	}
	var categoryRows []struct {
		CatalogID int32
		Title     string
	}
	err := s.db.WithContext(ctx).Raw(`SELECT p.id AS catalog_id, c.title
		FROM catalogs p JOIN catalog_categories c ON c.id = p.categories_id AND c.is_active = 1 AND c.title IS NOT NULL
		WHERE p.id IN ? UNION SELECT m.catalog_id, c.title
		FROM catalog_categories_multiple m JOIN catalog_categories c ON c.id = m.categories_id AND c.is_active = 1 AND c.title IS NOT NULL
		WHERE m.catalog_id IN ?`, ids, ids).Scan(&categoryRows).Error
	if err != nil {
		return nil, err
	}
	categories := map[int32][]string{}
	for _, row := range categoryRows {
		categories[row.CatalogID] = append(categories[row.CatalogID], row.Title)
	}
	facts := make([]Fact, 0, len(productRows))
	byID := make(map[int32]Fact, len(productRows))
	for _, row := range productRows {
		byID[row.ID] = Fact{CatalogID: row.ID, Title: textValue(row.Title), SKU: row.SKU,
			Price: row.Price, Stock: row.Stock, Promo: row.Promo, Categories: categories[row.ID]}
	}
	for _, entity := range entities {
		if fact, ok := byID[entity.CatalogID]; ok {
			facts = append(facts, fact)
		}
	}
	return facts, nil
}

func (s *Store) Category(ctx context.Context, name string) (string, error) {
	var row models.CatalogCategories
	err := s.db.WithContext(ctx).Where("is_active = 1 AND LOWER(title) = LOWER(?)", strings.TrimSpace(name)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return textValue(row.Title), nil
}

func (s *Store) Search(ctx context.Context, query string, sources []string, category string, catalogOnly bool, limit int) ([]Chunk, error) {
	vector, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(vector) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}
	value, err := models.Halfvec(vector).Value()
	if err != nil {
		return nil, err
	}
	statement := `SELECT chunk_id, source, field, content, metadata,
		embedding <=> ?::halfvec AS distance FROM rag_chunks
		WHERE embedding_model = ? AND embedding IS NOT NULL`
	args := []any{value, s.embeddingModel}
	if catalogOnly {
		statement += " AND source LIKE 'catalog:%'"
	}
	if len(sources) > 0 {
		statement += " AND source IN ?"
		args = append(args, sources)
	}
	if category != "" {
		categoryNames := []string{category}
		if strings.EqualFold(category, "Minuman") {
			var categories []models.CatalogCategories
			if err := s.db.WithContext(ctx).Model(&models.CatalogCategories{}).
				Select("title").Where("is_active = 1 AND title IS NOT NULL").Find(&categories).Error; err != nil {
				return nil, fmt.Errorf("load beverage categories: %w", err)
			}
			categoryNames = categoryNames[:0]
			for _, candidate := range categories {
				if candidate.Title != nil && isBeverageCategory(*candidate.Title) {
					categoryNames = append(categoryNames, *candidate.Title)
				}
			}
			if len(categoryNames) == 0 {
				return nil, nil
			}
		}
		filters := make([]string, 0, len(categoryNames))
		for _, name := range categoryNames {
			filter, err := json.Marshal(map[string][]string{"tags": {name}})
			if err != nil {
				return nil, err
			}
			filters = append(filters, "metadata @> ?::jsonb")
			args = append(args, string(filter))
		}
		statement += " AND (" + strings.Join(filters, " OR ") + ")"
	}
	statement += " ORDER BY embedding <=> ?::halfvec, source, chunk_id LIMIT ?"
	args = append(args, value, limit)
	var rows []struct {
		ChunkID  string
		Source   string
		Field    string
		Content  string
		Metadata []byte
		Distance float64
	}
	if err := s.db.WithContext(ctx).Raw(statement, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	chunks := make([]Chunk, 0, len(rows))
	for _, row := range rows {
		var metadata map[string]any
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			return nil, err
		}
		chunks = append(chunks, Chunk{ChunkID: row.ChunkID, Source: row.Source,
			Field: row.Field, Content: row.Content, Metadata: metadata, Distance: row.Distance})
	}
	return chunks, nil
}

func isBeverageCategory(name string) bool {
	for _, word := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !unicode.IsLetter(r) }) {
		switch word {
		case "minuman", "teh", "kopi", "jus":
			return true
		}
		word = strings.TrimSuffix(word, "s")
		switch word {
		case "drink", "beverage", "mocktail", "cocktail", "tea", "coffee",
			"juice", "smoothie", "milkshake", "soda", "latte", "matcha", "frappe",
			"lemonade", "water":
			return true
		}
	}
	return false
}

// traceRows flattens the workflow trace for the jsonb column. The database
// keeps a plain document; the typed shape stays inside the workflow.
func traceRows(steps []TraceStep) []any {
	rows := make([]any, 0, len(steps))
	for _, step := range steps {
		row := map[string]any{"node": step.Node, "ms": step.Millis}
		if len(step.Details) > 0 {
			row["details"] = step.Details
		}
		rows = append(rows, row)
	}
	return rows
}

func (s *Store) Persist(ctx context.Context, state State) error {
	return s.db.WithContext(ctx).Create(&models.RagTurn{RequestID: state.RequestID,
		ConversationID: state.ConversationID, UserMessage: state.UserMessage,
		Answer: state.Answer, Intent: string(state.Intent), ModelUsed: state.ModelUsed,
		Sources: state.SelectedSources, Citations: state.Citations,
		Trace:          traceRows(state.Trace),
		FallbackReason: state.FallbackReason}).Error
}
