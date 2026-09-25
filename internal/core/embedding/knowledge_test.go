package embedding

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadKnowledgeFileAndSplit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catatan.md")
	if err := os.WriteFile(path, []byte("# Tentang menu\n\n"+strings.Repeat("Menu ini dibuat segar. ", 100)), 0600); err != nil {
		t.Fatal(err)
	}
	docs, err := readKnowledgeFile(context.Background(), path, "catatan.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Metadata["path"] != "catatan.md" {
		t.Fatalf("unexpected documents: %+v", docs)
	}
	splitter, err := NewTokenSplitter(80, 10)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := splitter.Split(docs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		if !strings.HasPrefix(chunk, "Dokumen: catatan.md") {
			t.Fatalf("missing document header: %q", chunk)
		}
		count, err := splitter.count(chunk)
		if err != nil || count > 80 {
			t.Fatalf("chunk token count %d: %v", count, err)
		}
	}
}

func TestReadKnowledgeFormats(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "menu.csv")
	if err := os.WriteFile(csvPath, []byte("name,description\nOrange Tea,Fresh orange with tea\n"), 0600); err != nil {
		t.Fatal(err)
	}
	docs, err := readKnowledgeFile(context.Background(), csvPath, "menu.csv")
	if err != nil || len(docs) != 1 || !strings.Contains(docs[0].PageContent, "description: Fresh orange with tea") {
		t.Fatalf("CSV docs=%+v err=%v", docs, err)
	}

	docxPath := filepath.Join(dir, "guide.docx")
	writeKnowledgeZip(t, docxPath, map[string]string{"word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Jam operasional</w:t></w:r></w:p><w:p><w:r><w:t>Setiap hari</w:t></w:r></w:p></w:body></w:document>`})
	docs, err = readKnowledgeFile(context.Background(), docxPath, "guide.docx")
	if err != nil || len(docs) != 1 || !strings.Contains(docs[0].PageContent, "Jam operasional\n\nSetiap hari") {
		t.Fatalf("DOCX docs=%+v err=%v", docs, err)
	}

	xlsxPath := filepath.Join(dir, "menus.xlsx")
	writeKnowledgeZip(t, xlsxPath, map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Menu" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst><si><t>name</t></si><si><t>description</t></si><si><t>Orange Tea</t></si><si><t>Fresh orange</t></si></sst>`,
		"xl/worksheets/sheet1.xml":   `<worksheet><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row><row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2" t="s"><v>3</v></c></row></sheetData></worksheet>`,
	})
	docs, err = readKnowledgeFile(context.Background(), xlsxPath, "menus.xlsx")
	if err != nil || len(docs) != 1 || !strings.Contains(docs[0].PageContent, "description: Fresh orange") || docs[0].Metadata["sheet"] != "Menu" {
		t.Fatalf("XLSX docs=%+v err=%v", docs, err)
	}
}

func writeKnowledgeZip(t *testing.T, filename string, contents map[string]string) {
	t.Helper()
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, content := range contents {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
