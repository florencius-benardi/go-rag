package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go-rag/internal/configs"
)

type fakeModel struct{ classification string }

func (f fakeModel) Model() string { return "test-model" }
func (f fakeModel) Complete(_ context.Context, system, _ string) (string, error) {
	if strings.HasPrefix(system, "Return only JSON") {
		return f.classification, nil
	}
	return "Jawaban berdasarkan katalog.", nil
}

type historyCheckingModel struct {
	classification string
	calls          int
}

type unavailableModel struct{}

func (unavailableModel) Model() string { return "test-model" }
func (unavailableModel) Complete(context.Context, string, string) (string, error) {
	return "", fmt.Errorf("provider unavailable")
}

func (m *historyCheckingModel) Model() string { return "test-model" }
func (m *historyCheckingModel) Complete(_ context.Context, system, user string) (string, error) {
	m.calls++
	if !strings.Contains(user, `"content":"Halo"`) {
		return "", fmt.Errorf("history missing from LLM call: %s", user)
	}
	if strings.HasPrefix(system, "Return only JSON") {
		return m.classification, nil
	}
	return "Jawaban berdasarkan katalog.", nil
}

// traced reports whether a node ran, now that the trace carries timings too.
func traced(state State, node string) bool {
	for _, step := range state.Trace {
		if step.Node == node {
			return true
		}
	}
	return false
}

type fakeRepository struct{ saved State }

type recordingSearchRepo struct {
	fakeRepository
	query    string
	category string
}

type mocktailsOnlyRepo struct{ recordingSearchRepo }

func (r *mocktailsOnlyRepo) Category(context.Context, string) (string, error) {
	return "", nil // The catalog has MOCKTAILS, but no category named Minuman.
}

func (r *recordingSearchRepo) Search(_ context.Context, query string, _ []string, category string, _ bool, _ int) ([]Chunk, error) {
	r.query, r.category = query, category
	return []Chunk{{ChunkID: "drink-1", Source: "catalog:7", Content: "Produk: Es Lemon\n\nMinuman asam dan segar", Distance: 0.1}}, nil
}

func (f *fakeRepository) Recent(context.Context, string) ([]Message, error) {
	return []Message{{Role: "user", Content: "Halo"}}, nil
}
func (f *fakeRepository) Resolve(_ context.Context, names []string) ([]Entity, bool, error) {
	if len(names) == 0 {
		return nil, false, nil
	}
	return []Entity{{CatalogID: 7, Title: "Es Teh"}}, false, nil
}
func (f *fakeRepository) Facts(_ context.Context, entities []Entity) ([]Fact, error) {
	if len(entities) == 0 {
		return nil, nil
	}
	price := int32(12000)
	return []Fact{{CatalogID: 7, Title: "Es Teh", SKU: "ET-7", Price: &price}}, nil
}
func (f *fakeRepository) Search(context.Context, string, []string, string, bool, int) ([]Chunk, error) {
	return []Chunk{{ChunkID: "chunk-7", Source: "catalog:7", Content: "Produk: Es Teh\nBagian: Deskripsi\n\nSegar", Distance: 0.1}}, nil
}
func (f *fakeRepository) Category(context.Context, string) (string, error) { return "Minuman", nil }
func (f *fakeRepository) Persist(_ context.Context, state State) error     { f.saved = state; return nil }

func TestGraphRoutesByIntent(t *testing.T) {
	cases := []struct {
		name, classification string
		want                 []string
		dont                 []string
	}{
		{"fact", `{"intent":"FACT_LOOKUP","entities":["Es Teh"]}`, []string{"load_facts", "compose_answer"}, []string{"retrieve_knowledge"}},
		{"knowledge", `{"intent":"KNOWLEDGE_QA","entities":["Es Teh"]}`, []string{"retrieve_knowledge", "compose_answer"}, []string{"load_facts"}},
		{"recommendation", `{"intent":"RECOMMENDATION","entities":[],"category":"Minuman"}`, []string{"retrieve_knowledge", "score_candidates", "load_facts"}, []string{"resolve_entities"}},
		{"compare", `{"intent":"COMPARE","entities":["Es Teh"]}`, []string{"load_facts", "retrieve_knowledge"}, nil},
		{"cross_category", `{"intent":"CROSS_CATEGORY","entities":["Es Teh"]}`, []string{"load_facts", "retrieve_knowledge"}, nil},
		{"clarify", `{"intent":"CLARIFY","entities":[]}`, []string{"clarify"}, []string{"retrieve_knowledge", "compose_answer"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{}
			state, err := NewGraph(repo, fakeModel{classification: tc.classification}).Run(context.Background(), "", "test")
			if err != nil {
				t.Fatal(err)
			}
			for _, node := range tc.want {
				if !traced(state, node) {
					t.Errorf("missing %s in %v", node, state.Trace)
				}
			}
			for _, node := range tc.dont {
				if traced(state, node) {
					t.Errorf("unexpected %s in %v", node, state.Trace)
				}
			}
			if !traced(state, "persist_turn") || state.Answer == "" || repo.saved.RequestID != state.RequestID {
				t.Fatalf("turn not persisted: %#v", state)
			}
		})
	}
}

func TestOffTopicQuestionNeverCallsAnswerModel(t *testing.T) {
	dir := t.TempDir()
	policy := "# Waiter\n```guardrail\n{\"off_topic_answer\":\"Maaf, saya tidak bisa memberikan informasi yang anda.\",\"input_rules\":[{\"pattern\":\"(?i)\\\\bpresiden\\\\b\"}]}\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "waiter.md"), []byte(policy), 0600); err != nil {
		t.Fatal(err)
	}
	rules, err := LoadGuardrails(dir)
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepository{}
	state, err := NewGraph(repo, fakeModel{classification: `{"intent":"CLARIFY","entities":[]}`}).
		WithGuardrails(rules).
		Run(context.Background(), "", "presiden hari ini")
	if err != nil {
		t.Fatal(err)
	}
	if state.Answer != "Maaf, saya tidak bisa memberikan informasi yang anda." {
		t.Fatalf("off-topic answer = %q", state.Answer)
	}
}

func TestClassifiedOutOfScopeReturnsGuardrailAnswer(t *testing.T) {
	state, err := NewGraph(&fakeRepository{}, fakeModel{classification: `{"intent":"OUT_OF_SCOPE","entities":[]}`}).Run(context.Background(), "", "siapa juara dunia")
	if err != nil {
		t.Fatal(err)
	}
	if state.Answer != DefaultGuardrails().OffTopicAnswer || state.FallbackReason != "out_of_scope" {
		t.Fatalf("unexpected out-of-scope state: answer=%q reason=%q", state.Answer, state.FallbackReason)
	}
}

func TestGraphLoadsSessionHistoryBeforeBothModelCalls(t *testing.T) {
	repo := &fakeRepository{}
	model := &historyCheckingModel{classification: `{"intent":"FACT_LOOKUP","entities":["Es Teh"]}`}
	state, err := NewGraph(repo, model).Run(context.Background(), "session-1", "Berapa harganya?")
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 2 || state.ConversationID != "session-1" || repo.saved.UserMessage != "Berapa harganya?" {
		t.Fatalf("history or persistence failed: calls=%d state=%#v", model.calls, state)
	}
}

func TestRecommendationRequestDoesNotAskForProductTitle(t *testing.T) {
	for _, question := range []string{
		"Saya tidak bilang menu yang tersedia, tetapi berikan recommendation menu minuman dengan rasa asam",
		"berikan recomendation dong minuman apa",
	} {
		t.Run(question, func(t *testing.T) {
			repo := &fakeRepository{}
			state, err := NewGraph(repo, fakeModel{classification: `{"intent":"CLARIFY","entities":[],"category":""}`}).Run(context.Background(), "session-1", question)
			if err != nil {
				t.Fatal(err)
			}
			if state.Intent == Clarify || strings.Contains(state.Answer, "Produk mana yang Anda maksud?") {
				t.Fatalf("recommendation was rejected: intent=%s answer=%q", state.Intent, state.Answer)
			}
		})
	}
}

func TestDrinkRecommendationUsesCategoryWhenClassifierOmitsIt(t *testing.T) {
	repo := &recordingSearchRepo{}
	state, err := NewGraph(repo, fakeModel{classification: `{"intent":"RECOMMENDATION","entities":[],"category":""}`}).Run(
		context.Background(), "session-1", "berikan rekomendasi minuman yang asam")
	if err != nil {
		t.Fatal(err)
	}
	if state.Category != "Minuman" || repo.category != "Minuman" {
		t.Fatalf("drink category lost: state=%q search=%q", state.Category, repo.category)
	}
}

func TestSourRecommendationSearchIncludesEnglishTasteTerms(t *testing.T) {
	repo := &recordingSearchRepo{}
	_, err := NewGraph(repo, fakeModel{classification: `{"intent":"RECOMMENDATION","entities":[],"category":"Minuman"}`}).Run(
		context.Background(), "session-1", "berikan rekomendasi minuman yang asam")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(repo.query, "sour") {
		t.Fatalf("retrieval query lacks English taste synonym: %q", repo.query)
	}
}

// An outage must not cost the guest the words they actually typed. "rekomendasi"
// is read from the message itself, so it is evidence, not an invented intent.
func TestUnavailableClassifierStillHonoursTypedRecommendation(t *testing.T) {
	repo := &fakeRepository{}
	state, err := NewGraph(repo, unavailableModel{}).Run(context.Background(), "session-1", "berikan rekomendasi minuman yang asam")
	if err != nil {
		t.Fatal(err)
	}
	if state.Intent != Recommendation || strings.Contains(state.Answer, "layanan AI") {
		t.Fatalf("typed recommendation was dropped during outage: intent=%s answer=%q", state.Intent, state.Answer)
	}
	if !traced(state, "retrieve_knowledge") || state.Answer == "" {
		t.Fatalf("degraded turn produced no catalog answer: %#v", state)
	}
}

// With nothing deterministic in the message there is no honest answer to give,
// so the outage stays visible instead of being papered over.
func TestUnavailableClassifierClarifiesWithoutDeterministicSignal(t *testing.T) {
	repo := &fakeRepository{}
	state, err := NewGraph(repo, unavailableModel{}).Run(context.Background(), "session-1", "Kalo makanan mentah yang disajikan namanya apa?")
	if err != nil {
		t.Fatal(err)
	}
	if state.FallbackReason != "classification_provider_unavailable" ||
		!strings.Contains(state.Answer, "layanan AI") {
		t.Fatalf("provider outage was hidden: reason=%q answer=%q", state.FallbackReason, state.Answer)
	}
}

// A category the catalog lacks must widen the search, not end the turn.
func TestUnknownCategoryDropsFilterInsteadOfClarifying(t *testing.T) {
	repo := &mocktailsOnlyRepo{}
	state, err := NewGraph(repo, fakeModel{classification: `{"intent":"KNOWLEDGE_QA","entities":[],"category":"Food"}`}).Run(
		context.Background(), "session-1", "Kalo makanan mentah yang disajikan namanya apa?")
	if err != nil {
		t.Fatal(err)
	}
	if state.Intent == Clarify || !traced(state, "retrieve_knowledge") {
		t.Fatalf("answerable question was abandoned: intent=%s trace=%v", state.Intent, state.Trace)
	}
	if state.Category != "" || repo.category != "" {
		t.Fatalf("unknown category still filtered the search: state=%q search=%q", state.Category, repo.category)
	}
	if state.FallbackReason != "category_ignored" {
		t.Fatalf("dropped filter was not recorded: reason=%q", state.FallbackReason)
	}
}

func TestDrinkRecommendationSearchesWhenCatalogUsesMocktailsCategory(t *testing.T) {
	repo := &mocktailsOnlyRepo{}
	state, err := NewGraph(repo, fakeModel{classification: `{"intent":"RECOMMENDATION","entities":[],"category":"Minuman"}`}).Run(
		context.Background(), "session-1", "rekomendasikan minuman yang asam")
	if err != nil {
		t.Fatal(err)
	}
	if state.Intent == Clarify || repo.query == "" || repo.category != "Minuman" {
		t.Fatalf("mocktails were skipped: intent=%s search_query=%q search_category=%q answer=%q",
			state.Intent, repo.query, repo.category, state.Answer)
	}
}

func TestBeverageCategoryIncludesMocktailsButNotFood(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{
		{"MOCKTAILS", true},
		{"TEA & COFFEE", true},
		{"JUS SEGAR", true},
		{"MINUMAN", true},
		{"MENTAI GALORE", false},
		{"Vegetarian Menu", false},
	} {
		if got := isBeverageCategory(test.name); got != test.want {
			t.Errorf("isBeverageCategory(%q) = %t, want %t", test.name, got, test.want)
		}
	}
}

// capturingModel records the answer prompt so the payload itself can be asserted.
type capturingModel struct {
	classification string
	answerPrompt   string
}

func (m *capturingModel) Model() string { return "test-model" }
func (m *capturingModel) Complete(_ context.Context, system, user string) (string, error) {
	if strings.HasPrefix(system, "Return only JSON") {
		return m.classification, nil
	}
	m.answerPrompt = user
	return "Jawaban berdasarkan katalog.", nil
}

// A recommendation must not carry commercial data into the prompt. Turn 1 of the
// recorded session printed SKU, price, stock and chunk IDs in a table because the
// prompt forbade what the payload still contained.
func TestRecommendationPromptHidesCommercialAndRetrievalFields(t *testing.T) {
	model := &capturingModel{classification: `{"intent":"RECOMMENDATION","entities":[],"category":""}`}
	if _, err := NewGraph(&fakeRepository{}, model).Run(context.Background(), "session-1", "berikan menu makan segar"); err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"ET-7", "12000", `"sku"`, `"price"`, `"stock"`, "chunk-7", `"distance"`, `"chunk_id"`} {
		if strings.Contains(model.answerPrompt, leaked) {
			t.Errorf("recommendation prompt leaked %q:\n%s", leaked, model.answerPrompt)
		}
	}
	if !strings.Contains(model.answerPrompt, "Es Teh") {
		t.Fatalf("product title missing from prompt:\n%s", model.answerPrompt)
	}
}

// A guest who asks about stock or price must still be answered from real data.
func TestFactLookupPromptKeepsCommercialFields(t *testing.T) {
	model := &capturingModel{classification: `{"intent":"FACT_LOOKUP","entities":["Es Teh"]}`}
	if _, err := NewGraph(&fakeRepository{}, model).Run(context.Background(), "session-1", "Berapa harga Es Teh?"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ET-7", "12000", `"stock"`} {
		if !strings.Contains(model.answerPrompt, want) {
			t.Errorf("fact lookup prompt dropped %q:\n%s", want, model.answerPrompt)
		}
	}
}

// A price question outside FACT_LOOKUP still unlocks the commercial fields.
func TestExplicitPriceQuestionUnlocksCommercialFields(t *testing.T) {
	for _, question := range []string{"berapa harga minuman itu", "apakah masih tersedia", "stok nya bagaimana"} {
		state := &State{Intent: Recommendation, UserMessage: question}
		if !wantsCommercialDetail(state) {
			t.Errorf("commercial question was treated as casual: %q", question)
		}
	}
	if wantsCommercialDetail(&State{Intent: Recommendation, UserMessage: "berikan menu makan segar"}) {
		t.Error("casual recommendation was treated as a price question")
	}
}

func TestFallbackAnswerFollowsTheSameRulesAsThePrompt(t *testing.T) {
	price := int32(39000)
	many := []Fact{
		{Title: "Mix Berries", SKU: "GM2025", Price: &price},
		{Title: "Fruity Green Apple", SKU: "75055", Price: &price},
		{Title: "Sushi Tei Sunset", SKU: "75024", Price: &price},
		{Title: "Sushi Tei Sunrise", SKU: "75025", Price: &price},
		{Title: "Orenji Squash", SKU: "75050", Price: &price},
	}
	casual := fallbackAnswer(many, false, defaultAnswerMax)
	for _, leaked := range []string{"SKU", "GM2025", "39000", "Sushi Tei Sunrise", "Orenji Squash"} {
		if strings.Contains(casual, leaked) {
			t.Errorf("casual fallback leaked %q: %s", leaked, casual)
		}
	}
	if !strings.Contains(casual, "Mix Berries") || !strings.HasSuffix(casual, ".") {
		t.Errorf("casual fallback is not a plain sentence: %s", casual)
	}
	if detailed := fallbackAnswer(many[:1], true, defaultAnswerMax); !strings.Contains(detailed, "39000") || strings.Contains(detailed, "SKU") {
		t.Errorf("detailed fallback should quote price but never SKU: %s", detailed)
	}
	if empty := fallbackAnswer(nil, false, defaultAnswerMax); !strings.Contains(empty, "belum dapat menyusun") {
		t.Errorf("empty fallback lost its message: %s", empty)
	}
}

// stockedRepo serves a catalog of numbered products ranked by id, where only the
// odd ids are in stock unless the whole catalog is marked out of stock.
type stockedRepo struct {
	fakeRepository
	products      int
	allOutOfStock bool
	searchLimit   int
}

func (r *stockedRepo) Search(_ context.Context, _ string, _ []string, _ string, _ bool, limit int) ([]Chunk, error) {
	r.searchLimit = limit
	chunks := make([]Chunk, 0, r.products)
	for id := 1; id <= r.products; id++ {
		chunks = append(chunks, Chunk{
			ChunkID:  fmt.Sprintf("chunk-%d", id),
			Source:   fmt.Sprintf("catalog:%d", id),
			Content:  fmt.Sprintf("Produk: Menu %d\n\nSegar", id),
			Distance: float64(id) / 1000,
		})
	}
	return chunks, nil
}

func (r *stockedRepo) Facts(_ context.Context, entities []Entity) ([]Fact, error) {
	facts := make([]Fact, 0, len(entities))
	for _, entity := range entities {
		stock := int32(0)
		if !r.allOutOfStock && entity.CatalogID%2 == 1 {
			stock = 5
		}
		facts = append(facts, Fact{CatalogID: entity.CatalogID,
			Title: fmt.Sprintf("Menu %d", entity.CatalogID), Stock: stock})
	}
	return facts, nil
}

func entityIDs(entities []Entity) []int32 {
	ids := make([]int32, 0, len(entities))
	for _, entity := range entities {
		ids = append(ids, entity.CatalogID)
	}
	return ids
}

func TestRequestedCountReadsPlausibleQuantitiesOnly(t *testing.T) {
	for message, want := range map[string]int{
		"berikan 10 menu minuman yang segar": 10,
		"berikan 3 rekomendasi":              3,
		"berikan menu minuman buah":          0,
		"berikan 50 menu":                    0, // above the cap: not a count
		"berapa harga sku 75055":             0, // an SKU, not a count
	} {
		if got := requestedCount(message); got != want {
			t.Errorf("requestedCount(%q) = %d, want %d", message, got, want)
		}
	}
}

// Turn 4 of the recorded session asked for 10 and could never have received
// more than 3, because every stage capped the count independently.
func TestRequestedQuantityReachesRetrievalAndShortlist(t *testing.T) {
	repo := &stockedRepo{products: 30}
	state, err := NewGraph(repo, fakeModel{classification: `{"intent":"RECOMMENDATION","entities":[],"category":""}`}).Run(
		context.Background(), "session-1", "berikan 10 menu minuman yang segar")
	if err != nil {
		t.Fatal(err)
	}
	if state.RequestedCount != 10 {
		t.Fatalf("quantity was not read from the message: %d", state.RequestedCount)
	}
	if len(state.Entities) != 10 {
		t.Fatalf("shortlist ignored the requested quantity: %v", entityIDs(state.Entities))
	}
	if repo.searchLimit < 40 {
		t.Errorf("retrieval window too narrow for 10 products: limit=%d", repo.searchLimit)
	}
}

func TestDefaultRecommendationKeepsFiveCandidates(t *testing.T) {
	repo := &stockedRepo{products: 30, allOutOfStock: true}
	state, err := NewGraph(repo, fakeModel{classification: `{"intent":"RECOMMENDATION","entities":[],"category":""}`}).Run(
		context.Background(), "session-1", "berikan menu makan segar")
	if err != nil {
		t.Fatal(err)
	}
	if state.RequestedCount != 0 || len(state.Entities) != defaultCandidates {
		t.Fatalf("default shortlist changed: count=%d entities=%v", state.RequestedCount, entityIDs(state.Entities))
	}
}

// Turn 1 recommended five products that were all out of stock. Availability now
// reorders the shortlist before it is cut.
func TestInStockProductsOutrankSoldOutOnes(t *testing.T) {
	repo := &stockedRepo{products: 6}
	state, err := NewGraph(repo, fakeModel{classification: `{"intent":"RECOMMENDATION","entities":[],"category":""}`}).Run(
		context.Background(), "session-1", "berikan menu makan segar")
	if err != nil {
		t.Fatal(err)
	}
	got := entityIDs(state.Entities)
	want := []int32{1, 3, 5, 2, 4} // odd ids are in stock and keep their relative order
	if !slices.Equal(got, want) {
		t.Fatalf("availability did not reorder the shortlist: got %v want %v", got, want)
	}
}

// Some deployments leave stock at 0 for the whole catalog. Excluding those would
// answer nothing, so availability must only ever reorder.
func TestSoldOutCatalogStillProducesRecommendations(t *testing.T) {
	repo := &stockedRepo{products: 6, allOutOfStock: true}
	state, err := NewGraph(repo, fakeModel{classification: `{"intent":"RECOMMENDATION","entities":[],"category":""}`}).Run(
		context.Background(), "session-1", "berikan menu makan segar")
	if err != nil {
		t.Fatal(err)
	}
	got := entityIDs(state.Entities)
	if !slices.Equal(got, []int32{1, 2, 3, 4, 5}) {
		t.Fatalf("sold-out catalog lost its ranking: %v", got)
	}
}

// namedProductsRepo returns several products but an answer that mentions only
// one, reproducing turn 12: two products named, ten sources cited.
type namedProductsRepo struct{ stockedRepo }

func (r *namedProductsRepo) Facts(_ context.Context, entities []Entity) ([]Fact, error) {
	facts := make([]Fact, 0, len(entities))
	for _, entity := range entities {
		facts = append(facts, Fact{CatalogID: entity.CatalogID,
			Title: fmt.Sprintf("Menu %d", entity.CatalogID), Stock: 5})
	}
	return facts, nil
}

type fixedAnswerModel struct {
	classification string
	answer         string
}

func (m fixedAnswerModel) Model() string { return "test-model" }
func (m fixedAnswerModel) Complete(_ context.Context, system, _ string) (string, error) {
	if strings.HasPrefix(system, "Return only JSON") {
		return m.classification, nil
	}
	return m.answer, nil
}

func TestRecommendationCitesOnlyTheProductsTheAnswerNames(t *testing.T) {
	repo := &namedProductsRepo{stockedRepo{products: 6}}
	model := fixedAnswerModel{
		classification: `{"intent":"RECOMMENDATION","entities":[],"category":""}`,
		answer:         "Saya sarankan **Menu 2** dan Menu 4, keduanya terasa segar.",
	}
	state, err := NewGraph(repo, model).Run(context.Background(), "session-1", "berikan menu makan segar")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.SelectedSources) != defaultCandidates {
		t.Fatalf("shortlist changed, test premise broken: %v", state.SelectedSources)
	}
	want := []string{"catalog:2", "catalog:4", "chunk-2", "chunk-4"}
	if !slices.Equal(state.Citations, want) {
		t.Fatalf("citations do not match the answer: got %v want %v", state.Citations, want)
	}
}

// A guest who names the product is cited even when the answer paraphrases, as
// in "Stok Mix Berries saat ini 0" phrased without repeating the title.
func TestFactLookupCitesResolvedProductRegardlessOfPhrasing(t *testing.T) {
	model := fixedAnswerModel{
		classification: `{"intent":"FACT_LOOKUP","entities":["Es Teh"]}`,
		answer:         "Produk itu sudah habis.",
	}
	state, err := NewGraph(&fakeRepository{}, model).Run(context.Background(), "session-1", "Apakah stoknya ada?")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(state.Citations, []string{"catalog:7"}) {
		t.Fatalf("resolved product lost its citation: %v", state.Citations)
	}
}

// A knowledge question rests on the chunks themselves, so they stand as evidence.
func TestKnowledgeQuestionCitesItsChunksWhenThereAreNoFacts(t *testing.T) {
	model := fixedAnswerModel{
		classification: `{"intent":"KNOWLEDGE_QA","entities":[],"category":""}`,
		answer:         "Namanya sashimi.",
	}
	state, err := NewGraph(&fakeRepository{}, model).Run(context.Background(), "session-1", "Kalo makanan mentah namanya apa?")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Facts) != 0 || !slices.Equal(state.Citations, []string{"chunk-7"}) {
		t.Fatalf("knowledge evidence lost: facts=%d citations=%v", len(state.Facts), state.Citations)
	}
}

func TestTraceRecordsOutcomesNotJustNodeNames(t *testing.T) {
	repo := &stockedRepo{products: 6}
	state, err := NewGraph(repo, fakeModel{classification: `{"intent":"RECOMMENDATION","entities":[],"category":""}`}).Run(
		context.Background(), "session-1", "berikan 4 menu minuman yang segar")
	if err != nil {
		t.Fatal(err)
	}
	steps := map[string]TraceStep{}
	for _, step := range state.Trace {
		steps[step.Node] = step
		if step.Millis < 0 {
			t.Errorf("node %s recorded a negative duration: %d", step.Node, step.Millis)
		}
	}
	classify, ok := steps["classify_intent"]
	if !ok || classify.Details["intent"] != string(Recommendation) || classify.Details["requested_count"] != 4 {
		t.Fatalf("classification outcome not traced: %+v", classify)
	}
	retrieve, ok := steps["retrieve_knowledge"]
	if !ok || retrieve.Details["chunks"] != 6 || retrieve.Details["distance_min"] == nil {
		t.Fatalf("retrieval outcome not traced: %+v", retrieve)
	}
	if score := steps["score_candidates"]; score.Details["shortlisted"] != 4 {
		t.Fatalf("shortlist size not traced: %+v", score)
	}
}

// The trace is persisted as a plain jsonb document, not as Go types.
func TestTraceRowsFlattenForStorage(t *testing.T) {
	rows := traceRows([]TraceStep{
		{Node: "clarify", Millis: 3, Details: map[string]any{"fallback_reason": "unknown_intent"}},
		{Node: "persist_turn", Millis: 1},
	})
	if len(rows) != 2 {
		t.Fatalf("trace rows lost: %v", rows)
	}
	first, _ := rows[0].(map[string]any)
	second, _ := rows[1].(map[string]any)
	if first["node"] != "clarify" || first["ms"] != int64(3) {
		t.Fatalf("trace row lost its shape: %v", rows[0])
	}
	if _, ok := second["details"]; ok {
		t.Fatalf("empty details should be omitted: %v", rows[1])
	}
}

// requiredEnv makes configs.Load() succeed regardless of what else is set,
// so the Merchant assertions below are not at the mercy of the environment.
func requiredEnv(t *testing.T) {
	t.Helper()
	for key, value := range map[string]string{
		"DB_DATABASE": "test", "EMBEDDING_BASE_URL": "https://example.test",
		"EMBEDDING_API_KEY": "key", "EMBEDDING_MODEL": "model",
	} {
		t.Setenv(key, value)
	}
}

func TestMerchantIDDefaultsToNoOutletWhenUnset(t *testing.T) {
	requiredEnv(t)
	t.Setenv("MERCHANT_ID", "")
	cfg, err := configs.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Merchant.ID != 0 {
		t.Fatalf("MERCHANT_ID should default to 0 (no outlet configured), got %d", cfg.Merchant.ID)
	}
}

func TestMerchantIDReadsConfiguredOutlet(t *testing.T) {
	requiredEnv(t)
	t.Setenv("MERCHANT_ID", "7")
	cfg, err := configs.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Merchant.ID != 7 {
		t.Fatalf("MERCHANT_ID was not read from the environment: got %d", cfg.Merchant.ID)
	}
}

func TestPromoPriceIsLabelledNotStatedAsRegular(t *testing.T) {
	promoPrice := int32(25000)
	facts := []Fact{{CatalogID: 1, Title: "Es Teh", Price: &promoPrice, Promo: true}}
	view := factsForPrompt(facts, true)[0]
	if !view.Promo || view.Price == nil || *view.Price != promoPrice {
		t.Fatalf("promo price lost its flag going into the prompt: %+v", view)
	}

	answer := fallbackAnswer(facts, true, defaultAnswerMax)
	if !strings.Contains(answer, "harga promo") || strings.Contains(answer, "(harga 25000)") {
		t.Fatalf("fallback answer did not distinguish a promo price: %q", answer)
	}
}

func TestRegularPriceIsNeverLabelledAsPromo(t *testing.T) {
	price := int32(25000)
	facts := []Fact{{CatalogID: 1, Title: "Es Teh", Price: &price, Promo: false}}
	if view := factsForPrompt(facts, true)[0]; view.Promo {
		t.Fatalf("regular price was mislabelled as promo: %+v", view)
	}
	if answer := fallbackAnswer(facts, true, defaultAnswerMax); strings.Contains(answer, "promo") {
		t.Fatalf("regular price fallback mentioned promo: %q", answer)
	}
}

// Casual turns never show a price at all, promo or not — wantsCommercialDetail
// already governs that; Promo must not bypass it.
func TestCasualAnswerNeverLeaksPromoPrice(t *testing.T) {
	promoPrice := int32(25000)
	facts := []Fact{{CatalogID: 1, Title: "Es Teh", Price: &promoPrice, Promo: true}}
	answer := fallbackAnswer(facts, false, defaultAnswerMax)
	if strings.Contains(answer, "25000") || strings.Contains(answer, "promo") {
		t.Fatalf("casual fallback leaked promo pricing: %q", answer)
	}
}
