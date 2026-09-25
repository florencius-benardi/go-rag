package embedding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"go-rag/internal/domain/models"

	"github.com/google/uuid"
)

// SyncKnowledge indexes text documents under dir without changing the catalog
// index. Sources are stable relative paths, so a second run reuses embeddings.
func (e *Engine) SyncKnowledge(ctx context.Context, dir string) (SyncResult, error) {
	var result SyncResult
	root, err := filepath.Abs(dir)
	if err != nil {
		return result, err
	}
	if err := e.db.WithContext(ctx).Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return result, err
	}
	if err := e.db.WithContext(ctx).AutoMigrate(&models.RagChunks{}); err != nil {
		return result, err
	}
	rootKey, err := knowledgeRootKey(root)
	if err != nil {
		return result, err
	}
	sourcePrefix := "doc:" + rootKey + ":"
	trustedRules, err := filepath.Abs(filepath.Join("docs", "knowledge", "menyapa.md"))
	if err != nil {
		return result, err
	}
	seen := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".txt" && ext != ".pdf" && ext != ".docx" && ext != ".xlsx" && ext != ".csv" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		// This particular file is trusted chatbot policy, not searchable factual knowledge.
		if strings.EqualFold(path, trustedRules) || isGuardrailFile(path) {
			result.Skipped++
			return nil
		}
		source := sourcePrefix + rel
		seen[source] = true
		docs, err := readKnowledgeFile(ctx, path, rel)
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		chunks, embedded, reused, removed, err := e.syncKnowledgeFile(ctx, source, docs)
		if err != nil {
			return fmt.Errorf("sync %s: %w", rel, err)
		}
		result.Documents++
		result.Chunks += chunks
		result.Embedded += embedded
		result.Reused += reused
		result.Removed += removed
		return nil
	})
	if err != nil {
		return result, err
	}
	var sources []string
	if err := e.db.WithContext(ctx).Model(&models.RagChunks{}).Distinct("source").Where("LEFT(source, LENGTH(?)) = ?", sourcePrefix, sourcePrefix).Pluck("source", &sources).Error; err != nil {
		return result, err
	}
	for _, source := range sources {
		if seen[source] {
			continue
		}
		deleted := e.db.WithContext(ctx).Where("source = ?", source).Delete(&models.RagChunks{})
		if deleted.Error != nil {
			return result, deleted.Error
		}
		result.Removed += deleted.RowsAffected
	}
	return result, nil
}

func knowledgeRootKey(root string) (string, error) {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(root))))
	return hex.EncodeToString(sum[:10]), nil
}

func isGuardrailFile(path string) bool {
	guardrailDir, err := filepath.Abs(filepath.Join("docs", "guardrails"))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(guardrailDir, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func readKnowledgeFile(ctx context.Context, path, rel string) ([]Document, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".docx":
		return readDOCX(path, rel)
	case ".csv":
		return readCSV(path, rel)
	case ".xlsx":
		return readXLSX(path, rel)
	}
	var pages []string
	if strings.EqualFold(filepath.Ext(path), ".pdf") {
		output, err := exec.CommandContext(ctx, "pdftotext", "-layout", "-enc", "UTF-8", path, "-").Output()
		if err != nil {
			return nil, fmt.Errorf("pdftotext (install poppler for PDF support): %w", err)
		}
		pages = strings.Split(string(output), "\f")
	} else {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		pages = []string{string(content)}
	}
	docs := make([]Document, 0, len(pages))
	for i, page := range pages {
		page = strings.TrimSpace(strings.TrimPrefix(page, "\ufeff"))
		if page == "" {
			continue
		}
		metadata := map[string]any{"path": rel, "page": i + 1}
		header := "Dokumen: " + rel
		if strings.EqualFold(filepath.Ext(path), ".pdf") {
			header += "\nHalaman: " + strconv.Itoa(i+1)
		}
		docs = append(docs, Document{PageContent: page, Metadata: metadata, Field: fmt.Sprintf("page:%d", i+1), Header: header + "\n\n"})
	}
	return docs, nil
}

func (e *Engine) syncKnowledgeFile(ctx context.Context, source string, docs []Document) (chunks, embedded, reused int, removed int64, err error) {
	var existing []models.RagChunks
	if err = e.db.WithContext(ctx).Where("source = ?", source).Find(&existing).Error; err != nil {
		return
	}
	old := make(map[string]models.RagChunks, len(existing))
	for _, row := range existing {
		if row.ChunkID != nil {
			old[*row.ChunkID] = row
		}
	}
	keep := make([]string, 0)
	for _, doc := range docs {
		var parts []string
		parts, err = e.splitter.Split(doc)
		if err != nil {
			return
		}
		for i, content := range parts {
			id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(source+"/"+doc.Field+"/"+strconv.Itoa(i))).String()
			keep = append(keep, id)
			sum := sha256.Sum256([]byte(content))
			hash := hex.EncodeToString(sum[:])
			metadata := make(map[string]any, len(doc.Metadata)+2)
			for k, v := range doc.Metadata {
				metadata[k] = v
			}
			metadata["chunk_index"] = i
			metadata["chunk_count"] = len(parts)
			row := models.RagChunks{ChunkID: &id, Source: source, Field: doc.Field, Content: content, ContentHash: hash, EmbeddingModel: e.config.Model, Metadata: metadata}
			previous, found := old[id]
			if found {
				row.ID = previous.ID
			}
			if found && previous.ContentHash == hash && previous.EmbeddingModel == e.config.Model {
				reused++
			} else {
				var vector []float32
				vector, err = e.embedder.EmbedPassage(ctx, content)
				if err != nil {
					return
				}
				if len(vector) == 0 {
					err = fmt.Errorf("empty embedding for %s", id)
					return
				}
				row.Embedding = models.Halfvec(vector)
				embedded++
			}
			if found {
				updates := map[string]any{"metadata": row.Metadata}
				if row.Embedding != nil {
					updates["content"] = row.Content
					updates["content_hash"] = hash
					updates["embedding_model"] = row.EmbeddingModel
					updates["embedding"] = row.Embedding
				}
				err = e.db.WithContext(ctx).Model(&models.RagChunks{}).Where("id = ?", row.ID).Updates(updates).Error
			} else {
				err = e.db.WithContext(ctx).Create(&row).Error
			}
			if err != nil {
				return
			}
			chunks++
		}
	}
	query := e.db.WithContext(ctx).Where("source = ?", source)
	if len(keep) > 0 {
		query = query.Where("chunk_id IS NULL OR chunk_id NOT IN ?", keep)
	}
	deleted := query.Delete(&models.RagChunks{})
	removed, err = deleted.RowsAffected, deleted.Error
	return
}
