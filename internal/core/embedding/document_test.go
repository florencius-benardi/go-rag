package embedding

import (
	"reflect"
	"strings"
	"testing"
)

func TestCatalogDocumentsCombineNarrativeFields(t *testing.T) {
	title, intro, description := "Es Krim Cokelat", "Dessert dingin", "Dengan saus cokelat"
	price := int32(25000)
	docs := documentsForCatalog(catalog{ID: 42, Title: &title, Intro: &intro,
		Description: &description, Price: &price, SKU: "ICE-42",
		Categories: []category{{ID: 2, Name: "Dessert"}, {ID: 1, Name: "Food"}, {ID: 2, Name: "Dessert"}}})
	if len(docs) != 1 || docs[0].Field != "description" {
		t.Fatalf("documents = %#v", docs)
	}
	if !strings.Contains(docs[0].Header, "Kategori: Food, Dessert") ||
		!strings.Contains(docs[0].Header, title) ||
		docs[0].PageContent != "Ringkasan:\n"+intro+"\n\nDeskripsi:\n"+description {
		t.Fatalf("incorrect combined document: %#v", docs[0])
	}
	if docs[0].Metadata["price"] != price || docs[0].Metadata["sku"] != "ICE-42" ||
		!reflect.DeepEqual(docs[0].Metadata["tags"], []string{"Food", "Dessert"}) ||
		!reflect.DeepEqual(docs[0].Metadata["covered_fields"], []string{"intro", "description"}) {
		t.Fatalf("incorrect metadata: %#v", docs[0].Metadata)
	}
	if strings.Contains(docs[0].Header+docs[0].PageContent, "Harga:") ||
		strings.Contains(docs[0].Header+docs[0].PageContent, "SKU:") {
		t.Fatal("structured facts leaked into embedding text")
	}
}

func TestCatalogDocumentsRemoveHTMLAndEmptyParagraphs(t *testing.T) {
	title := "<p>Es <strong>Teh</strong></p>"
	intro := "<p>-</p><p>Minuman &amp; dessert</p>"
	description := "<p class=\"copy\">Segar<br>dingin</p><p> - </p><script>ignore me</script>"
	docs := documentsForCatalog(catalog{ID: 7, Title: &title, Intro: &intro,
		Description: &description, Categories: []category{{ID: 1, Name: "<p>Minuman</p>"}}})
	if len(docs) != 1 || strings.Contains(docs[0].PageContent, "<") ||
		strings.Contains(docs[0].PageContent, "ignore me") ||
		strings.Contains(docs[0].PageContent, "-") ||
		docs[0].PageContent != "Ringkasan:\nMinuman & dessert\n\nDeskripsi:\nSegar\ndingin" {
		t.Fatalf("HTML cleanup failed: %#v", docs)
	}
}

func TestCatalogDocumentsKeepSingleNarrativeField(t *testing.T) {
	title, intro := "Produk Uji", "Hanya ringkasan"
	docs := documentsForCatalog(catalog{ID: 10, Title: &title, Intro: &intro})
	if len(docs) != 1 || docs[0].Field != "intro" || docs[0].PageContent != intro {
		t.Fatalf("intro-only documents = %#v", docs)
	}
	if _, ok := docs[0].Metadata["covered_fields"]; ok {
		t.Fatalf("unexpected covered_fields: %#v", docs[0].Metadata)
	}
}

func TestCatalogDocumentsSkipPlainHyphenPlaceholder(t *testing.T) {
	title, intro, description := "Produk Uji", " - ", "-"
	docs := documentsForCatalog(catalog{ID: 8, Title: &title, Intro: &intro, Description: &description})
	if len(docs) != 0 {
		t.Fatalf("placeholder fields must not create documents: %#v", docs)
	}
}

func TestCatalogDocumentsDeduplicateIdenticalIntroAndDescription(t *testing.T) {
	title := "Shitake Gyubara"
	intro := "<p>rolled beef &amp; shitake mushroom grilled with sweet shoyu &amp; cod roe mayo skewer</p>"
	description := "rolled beef & shitake mushroom grilled with sweet shoyu & cod roe mayo skewer"
	docs := documentsForCatalog(catalog{ID: 9, Title: &title, Intro: &intro,
		Description: &description, Categories: []category{{ID: 4, Name: "YAKIMONO"}}})
	if len(docs) != 1 || docs[0].Field != "description" {
		t.Fatalf("expected one description document, got %#v", docs)
	}
	if got := docs[0].Metadata["covered_fields"]; !reflect.DeepEqual(got, []string{"intro", "description"}) {
		t.Fatalf("covered_fields = %#v", got)
	}
	if docs[0].PageContent != description || !strings.Contains(docs[0].Header, "Bagian: Deskripsi") {
		t.Fatalf("identical narrative changed: %#v", docs[0])
	}
	splitter, err := NewTokenSplitter(500, 75)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := splitter.Split(docs[0])
	if err != nil || len(chunks) != 1 {
		t.Fatalf("expected one chunk, got %d: %v", len(chunks), err)
	}
}

func TestCombinedCatalogNarrativeSplitsWithinTokenBudget(t *testing.T) {
	title, intro := "Produk Panjang", "Ringkasan singkat"
	description := strings.Repeat("Rasa cokelat lembut. ", 300)
	docs := documentsForCatalog(catalog{ID: 11, Title: &title, Intro: &intro, Description: &description,
		Categories: []category{{ID: 1, Name: "Dessert"}}})
	splitter, err := NewTokenSplitter(500, 75)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := splitter.Split(docs[0])
	if err != nil || len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d: %v", len(chunks), err)
	}
	for _, chunk := range chunks {
		count, err := splitter.count(chunk)
		if err != nil || count > 500 || !strings.HasPrefix(chunk, docs[0].Header) {
			t.Fatalf("invalid chunk (%d tokens): %q, err=%v", count, chunk, err)
		}
	}
}

func TestTokenSplitterRepeatsHeaderAndRespectsBudget(t *testing.T) {
	splitter, err := NewTokenSplitter(40, 5)
	if err != nil {
		t.Fatal(err)
	}
	doc := Document{Header: "Produk: Es Krim\nBagian: Deskripsi\n\n",
		PageContent: strings.Repeat("Rasa cokelat lembut. ", 30)}
	chunks, err := splitter.Split(doc)
	if err != nil || len(chunks) < 2 {
		t.Fatalf("chunks = %d, err = %v", len(chunks), err)
	}
	for _, chunk := range chunks {
		count, err := splitter.count(chunk)
		if err != nil || count > 40 || !strings.HasPrefix(chunk, doc.Header) {
			t.Fatalf("invalid chunk (%d tokens): %q, err=%v", count, chunk, err)
		}
	}
}
