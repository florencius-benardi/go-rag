package embedding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"go-rag/internal/configs"
	"go-rag/internal/domain/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const catalogPageSize = 100

type Engine struct {
	db       *gorm.DB
	embedder Embedder
	config   configs.EmbeddingRAGConfig
	splitter *TokenSplitter
}

type SyncResult struct {
	Products        int
	Documents       int
	Skipped         int
	Chunks          int
	Embedded        int
	MetadataUpdated int
	Reused          int
	Removed         int64
}

type catalogSyncResult struct {
	Chunks          int
	Embedded        int
	MetadataUpdated int
	Reused          int
	Deleted         int64
	PreviousPrice   *int32
}

func NewEngine(db *gorm.DB, embedder Embedder, cfg configs.EmbeddingRAGConfig) (*Engine, error) {
	if db == nil || embedder == nil {
		return nil, fmt.Errorf("database and embedder are required")
	}
	if cfg.ChunkSize < 1 || cfg.ChunkOverlap < 0 || cfg.ChunkOverlap >= cfg.ChunkSize {
		return nil, fmt.Errorf("invalid embedding chunk size or overlap")
	}
	splitter, err := NewTokenSplitter(cfg.ChunkSize, cfg.ChunkOverlap)
	if err != nil {
		return nil, err
	}
	return &Engine{db: db, embedder: embedder, config: cfg, splitter: splitter}, nil
}

// SyncCatalogs embeds only changed field chunks, then removes stale chunks.
func (e *Engine) SyncCatalogs(ctx context.Context) (SyncResult, error) {
	var result SyncResult
	if err := e.db.WithContext(ctx).Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return result, fmt.Errorf("enable pgvector extension: %w", err)
	}
	if err := e.db.WithContext(ctx).AutoMigrate(&models.RagChunks{}); err != nil {
		return result, fmt.Errorf("migrate rag_chunks: %w", err)
	}
	if err := e.db.WithContext(ctx).Exec(`CREATE INDEX IF NOT EXISTS idx_rag_chunks_metadata
		ON rag_chunks USING GIN (metadata)`).Error; err != nil {
		return result, fmt.Errorf("index rag_chunks metadata: %w", err)
	}

	var lastID int32
	for {
		products, err := e.loadCatalogPage(ctx, lastID)
		if err != nil {
			return result, err
		}
		if len(products) == 0 {
			break
		}
		for _, product := range products {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			catalogResult, err := e.syncCatalog(ctx, product)
			if err != nil {
				return result, fmt.Errorf("catalog %d: %w", product.ID, err)
			}
			result.Products++
			result.Chunks += catalogResult.Chunks
			result.Embedded += catalogResult.Embedded
			result.MetadataUpdated += catalogResult.MetadataUpdated
			result.Reused += catalogResult.Reused
			e.debugCatalogSync(product, catalogResult)
		}
		lastID = products[len(products)-1].ID
	}
	removed := e.db.WithContext(ctx).Exec(`
		DELETE FROM rag_chunks
		WHERE source LIKE 'catalog:%'
		AND source NOT IN (
			SELECT 'catalog:' || id::text FROM catalogs WHERE is_active = 1
		)
	`)
	if removed.Error != nil {
		return result, fmt.Errorf("remove inactive catalog chunks: %w", removed.Error)
	}
	result.Removed = removed.RowsAffected
	return result, nil
}

type catalogRow struct {
	ID             int32
	Title          *string
	Intro          *string
	Description    *string
	Price          *int32
	SKU            string
	UpdatedAt      *time.Time `gorm:"column:updated_at"`
	CategoriesJSON []byte     `gorm:"column:categories_json"`
}

func (e *Engine) loadCatalogPage(ctx context.Context, afterID int32) ([]catalog, error) {
	// The lateral aggregate preserves one row per product even when it has
	// multiple categories. It includes primary and additional active categories.
	const query = `
		SELECT p.id, p.title, p.intro, p.description, p.price, p.sku, p.updated_at,
			COALESCE(tags.categories_json, '[]'::jsonb) AS categories_json
		FROM catalogs AS p
		LEFT JOIN LATERAL (
			WITH RECURSIVE selected_categories AS (
				SELECT c.id, c.parent_id, c.title
				FROM catalog_categories AS c
				WHERE c.is_active = 1 AND (
					c.id = p.categories_id OR EXISTS (
						SELECT 1 FROM catalog_categories_multiple AS cm
						WHERE cm.catalog_id = p.id AND cm.categories_id = c.id
					)
				)
				UNION
				SELECT parent.id, parent.parent_id, parent.title
				FROM catalog_categories AS parent
				JOIN selected_categories AS child ON child.parent_id = parent.id
				WHERE parent.is_active = 1
			)
			SELECT jsonb_agg(jsonb_build_object('id', id, 'name', title)) AS categories_json
			FROM selected_categories WHERE title IS NOT NULL
		) AS tags ON TRUE
		WHERE p.is_active = 1 AND p.id > ?
		ORDER BY p.id
		LIMIT ?`
	var rows []catalogRow
	if err := e.db.WithContext(ctx).Raw(query, afterID, catalogPageSize).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load active catalogs: %w", err)
	}
	products := make([]catalog, 0, len(rows))
	for _, row := range rows {
		var categories []category
		if err := json.Unmarshal(row.CategoriesJSON, &categories); err != nil {
			return nil, fmt.Errorf("decode catalog %d categories: %w", row.ID, err)
		}
		products = append(products, catalog{
			ID: row.ID, Title: row.Title, Intro: row.Intro,
			Description: row.Description, Price: row.Price, SKU: row.SKU, UpdatedAt: row.UpdatedAt,
			Categories: categories,
		})
	}
	return products, nil
}

func (e *Engine) syncCatalog(ctx context.Context, product catalog) (catalogSyncResult, error) {
	var result catalogSyncResult
	source := CatalogSource(product.ID)
	var existing []models.RagChunks
	if err := e.db.WithContext(ctx).Select("id", "chunk_id", "content_hash", "embedding_model", "metadata").Where("source = ?", source).Find(&existing).Error; err != nil {
		return result, fmt.Errorf("load existing chunks: %w", err)
	}
	result.PreviousPrice = existingPrice(existing)
	byID := make(map[string]models.RagChunks, len(existing))
	for _, row := range existing {
		if row.ChunkID != nil {
			byID[*row.ChunkID] = row
		}
	}
	rows := make([]models.RagChunks, 0)
	unchanged := make(map[string]bool)
	for _, doc := range documentsForCatalog(product) {
		chunks, err := e.splitter.Split(doc)
		if err != nil {
			return result, fmt.Errorf("split %s: %w", doc.Field, err)
		}
		for i, content := range chunks {
			chunkID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(source+"/"+doc.Field+"/"+strconv.Itoa(i))).String()
			sum := sha256.Sum256([]byte(content))
			hash := hex.EncodeToString(sum[:])
			metadata := make(map[string]any, len(doc.Metadata)+4)
			for key, value := range doc.Metadata {
				metadata[key] = value
			}
			metadata["chunk_index"] = i
			metadata["chunk_count"] = len(chunks)
			metadata["content_hash"] = hash
			metadata["embedding_model"] = e.config.Model
			row := models.RagChunks{ChunkID: &chunkID, Source: source, Field: doc.Field,
				ContentHash: hash, EmbeddingModel: e.config.Model, Content: content, Metadata: metadata}
			if previous, ok := byID[chunkID]; ok && previous.ContentHash == hash && previous.EmbeddingModel == e.config.Model {
				row.ID = previous.ID
				unchanged[chunkID] = true
				result.Reused++
			} else {
				vector, err := e.embedder.EmbedPassage(ctx, content)
				if err != nil {
					return result, err
				}
				if len(vector) == 0 {
					return result, fmt.Errorf("chunk %d has an empty embedding", i)
				}
				row.Embedding = models.Halfvec(vector)
				result.Embedded++
				if ok {
					row.ID = previous.ID
				}
			}
			rows = append(rows, row)
		}
	}
	if err := e.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keep := make([]string, 0, len(rows))
		for _, row := range rows {
			keep = append(keep, *row.ChunkID)
			if row.ID != 0 {
				if unchanged[*row.ChunkID] && equalMetadata(byID[*row.ChunkID].Metadata, row.Metadata) {
					continue
				}
				if unchanged[*row.ChunkID] {
					result.MetadataUpdated++
				}
				updates := map[string]any{"metadata": row.Metadata}
				if !unchanged[*row.ChunkID] {
					updates["content"] = row.Content
					updates["field"] = row.Field
					updates["content_hash"] = row.ContentHash
					updates["embedding_model"] = row.EmbeddingModel
					updates["embedding"] = row.Embedding
				}
				if err := tx.Model(&models.RagChunks{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
					return err
				}
			} else if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		query := tx.Where("source = ?", source)
		if len(keep) > 0 {
			query = query.Where("chunk_id IS NULL OR chunk_id NOT IN ?", keep)
		}
		deleted := query.Delete(&models.RagChunks{})
		result.Deleted = deleted.RowsAffected
		return deleted.Error
	}); err != nil {
		return result, fmt.Errorf("save chunks: %w", err)
	}
	result.Chunks = len(rows)
	return result, nil
}

func (e *Engine) debugCatalogSync(product catalog, result catalogSyncResult) {
	if !e.config.SyncDebug {
		return
	}
	updatedAt := "null"
	if product.UpdatedAt != nil {
		updatedAt = product.UpdatedAt.UTC().Format(time.RFC3339)
	}
	priceChanged := !samePrice(product.Price, result.PreviousPrice)
	action := "unchanged"
	if result.Embedded > 0 {
		action = "reembedded_narrative"
	} else if result.MetadataUpdated > 0 {
		action = "metadata_only"
	} else if result.Deleted > 0 {
		action = "deleted_stale_chunks"
	}
	fmt.Printf("[embedding-sync] catalog_id=%d updated_at=%s price=%s previous_price=%s price_changed=%t chunks=%d embedded=%d reused=%d metadata_updated=%d deleted=%d action=%s\n",
		product.ID, updatedAt, formatPrice(product.Price), formatPrice(result.PreviousPrice), priceChanged,
		result.Chunks, result.Embedded, result.Reused, result.MetadataUpdated, result.Deleted, action)
}

func existingPrice(rows []models.RagChunks) *int32 {
	for _, row := range rows {
		value, ok := row.Metadata["price"]
		if !ok {
			continue
		}
		price, ok := toInt32(value)
		if ok {
			return &price
		}
	}
	return nil
}

func toInt32(value any) (int32, bool) {
	switch v := value.(type) {
	case int32:
		return v, true
	case int:
		return int32(v), true
	case float64:
		return int32(v), v == float64(int32(v))
	case json.Number:
		parsed, err := strconv.ParseInt(string(v), 10, 32)
		return int32(parsed), err == nil
	default:
		return 0, false
	}
}

func samePrice(current, previous *int32) bool {
	return (current == nil && previous == nil) || (current != nil && previous != nil && *current == *previous)
}

func formatPrice(value *int32) string {
	if value == nil {
		return "null"
	}
	return strconv.FormatInt(int64(*value), 10)
}

func equalMetadata(a, b map[string]any) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}

// CatalogSource returns the stable source key used in rag_chunks.
func CatalogSource(id int32) string {
	return fmt.Sprintf("catalog:%d", id)
}
