package rag

type Intent string

const (
	FactLookup     Intent = "FACT_LOOKUP"
	KnowledgeQA    Intent = "KNOWLEDGE_QA"
	Recommendation Intent = "RECOMMENDATION"
	Compare        Intent = "COMPARE"
	CrossCategory  Intent = "CROSS_CATEGORY"
	Clarify        Intent = "CLARIFY"
	OutOfScope     Intent = "OUT_OF_SCOPE"
)

// TraceStep records what a node did, not merely that it ran. The recorded
// sessions could not be diagnosed from a list of node names alone.
type TraceStep struct {
	Node    string         `json:"node"`
	Millis  int64          `json:"ms"`
	Details map[string]any `json:"details,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Entity struct {
	CatalogID int32  `json:"catalog_id"`
	Title     string `json:"title"`
}

type Fact struct {
	CatalogID int32  `json:"catalog_id"`
	Title     string `json:"title"`
	SKU       string `json:"sku"`
	// Price and Stock already reflect the configured merchant's override, when
	// one exists for this product; Store.Facts resolves that, not the caller.
	Price *int32 `json:"price,omitempty"`
	Stock int32  `json:"stock"`
	// Promo is true when Price came from a merchant override that differs from
	// the catalog's own price, so an answer can say it is a promo instead of
	// stating it as the product's regular price.
	Promo      bool     `json:"promo,omitempty"`
	Categories []string `json:"categories"`
}

type Chunk struct {
	ChunkID  string         `json:"chunk_id"`
	Source   string         `json:"source"`
	Field    string         `json:"field"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
	Distance float64        `json:"distance"`
}

type Candidate struct {
	Source string  `json:"source"`
	Score  float64 `json:"score"`
}

// State is the explicit contract shared by every workflow node.
type State struct {
	RequestID       string      `json:"request_id"`
	ConversationID  string      `json:"conversation_id"`
	UserMessage     string      `json:"user_message"`
	RecentMessages  []Message   `json:"recent_messages"`
	Intent          Intent      `json:"intent"`
	Category        string      `json:"category,omitempty"`
	RequestedCount  int         `json:"requested_count,omitempty"`
	Entities        []Entity    `json:"entities"`
	SelectedSources []string    `json:"selected_sources"`
	Facts           []Fact      `json:"facts"`
	RetrievedChunks []Chunk     `json:"retrieved_chunks"`
	Candidates      []Candidate `json:"candidates,omitempty"`
	Answer          string      `json:"answer"`
	Citations       []string    `json:"citations"`
	ModelUsed       string      `json:"model_used"`
	FallbackReason  string      `json:"fallback_reason,omitempty"`
	Trace           []TraceStep `json:"trace"`
}

type Classification struct {
	Intent   Intent   `json:"intent"`
	Entities []string `json:"entities"`
	Category string   `json:"category"`
}
