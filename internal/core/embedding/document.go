package embedding

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Document keeps one narrative field and its structured product metadata.
type Document struct {
	PageContent string
	Metadata    map[string]any
	Field       string
	Header      string
}

type category struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

type catalog struct {
	ID          int32
	Title       *string
	Intro       *string
	Description *string
	Price       *int32
	SKU         string
	UpdatedAt   *time.Time
	Categories  []category
}

func documentsForCatalog(p catalog) []Document {
	categories := append([]category(nil), p.Categories...)
	sort.Slice(categories, func(i, j int) bool { return categories[i].ID < categories[j].ID })
	names := make([]string, 0, len(categories))
	ids := make([]int32, 0, len(categories))
	seen := make(map[int32]bool, len(categories))
	for _, c := range categories {
		name := cleanCatalogText(c.Name)
		if seen[c.ID] || name == "" {
			continue
		}
		seen[c.ID] = true
		ids = append(ids, c.ID)
		names = append(names, name)
	}
	metadata := map[string]any{
		"source":       fmt.Sprintf("catalog:%d", p.ID),
		"catalog_id":   p.ID,
		"title":        optionalText(p.Title),
		"sku":          p.SKU,
		"categories":   names,
		"category_ids": ids,
		"tags":         names,
	}
	if p.Price != nil {
		metadata["price"] = *p.Price
	}
	intro := optionalText(p.Intro)
	description := optionalText(p.Description)
	duplicateNarrative := intro != "" && intro == description
	var documents []Document
	for _, section := range []struct{ field, label, content string }{
		{"intro", "Ringkasan", intro},
		{"description", "Deskripsi", description},
	} {
		if section.content == "" || (duplicateNarrative && section.field == "intro") {
			continue
		}
		header := "Produk: " + optionalText(p.Title)
		if len(names) > 0 {
			header += "\nKategori: " + strings.Join(names, ", ")
		}
		header += "\nBagian: " + section.label + "\n\n"
		copyMetadata := make(map[string]any, len(metadata)+1)
		for key, value := range metadata {
			copyMetadata[key] = value
		}
		copyMetadata["field"] = section.field
		if duplicateNarrative {
			copyMetadata["covered_fields"] = []string{"intro", "description"}
		}
		documents = append(documents, Document{PageContent: section.content, Metadata: copyMetadata, Field: section.field, Header: header})
	}
	return documents
}

func optionalText(value *string) string {
	if value == nil {
		return ""
	}
	return cleanCatalogText(*value)
}

// cleanCatalogText converts product HTML to readable text. Empty paragraphs
// containing only a hyphen are placeholders and do not belong in embeddings.
func cleanCatalogText(value string) string {
	nodes, err := html.ParseFragment(strings.NewReader(value), &html.Node{
		Type: html.ElementNode, DataAtom: atom.Div, Data: "div",
	})
	if err != nil {
		return strings.TrimSpace(value)
	}

	var out strings.Builder
	var writeNode func(*html.Node)
	writeNode = func(node *html.Node) {
		switch node.Type {
		case html.TextNode:
			out.WriteString(node.Data)
		case html.ElementNode:
			switch node.Data {
			case "script", "style":
				return
			case "p":
				if strings.TrimSpace(nodeText(node)) == "-" {
					return
				}
			case "br":
				out.WriteByte('\n')
				return
			}
			block := isBlockElement(node.Data)
			if block {
				out.WriteByte('\n')
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				writeNode(child)
			}
			if block {
				out.WriteByte('\n')
			}
		}
	}
	for _, node := range nodes {
		writeNode(node)
	}

	lines := strings.Split(out.String(), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	result := strings.Join(cleaned, "\n")
	if result == "-" {
		return ""
	}
	return result
}

func nodeText(node *html.Node) string {
	var out strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			out.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return out.String()
}

func isBlockElement(name string) bool {
	switch name {
	case "p", "div", "li", "ul", "ol", "h1", "h2", "h3", "h4", "h5", "h6", "section", "article":
		return true
	default:
		return false
	}
}
