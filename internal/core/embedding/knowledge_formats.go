package embedding

import (
	"archive/zip"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
)

func readDOCX(filename, rel string) ([]Document, error) {
	archive, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	file := zipFile(archive.File, "word/document.xml")
	if file == nil {
		return nil, fmt.Errorf("DOCX has no word/document.xml")
	}
	stream, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	decoder := xml.NewDecoder(stream)
	var paragraphs []string
	var paragraph strings.Builder
	insideText := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "t":
				insideText = true
			case "tab":
				paragraph.WriteByte('\t')
			case "br":
				paragraph.WriteByte('\n')
			}
		case xml.CharData:
			if insideText {
				paragraph.Write(value)
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "t":
				insideText = false
			case "p":
				if text := strings.TrimSpace(paragraph.String()); text != "" {
					paragraphs = append(paragraphs, text)
				}
				paragraph.Reset()
			}
		}
	}
	if len(paragraphs) == 0 {
		return nil, nil
	}
	return []Document{{PageContent: strings.Join(paragraphs, "\n\n"), Field: "body",
		Header: "Dokumen: " + rel + "\n\n", Metadata: map[string]any{"path": rel, "format": "docx"}}}, nil
}

func readCSV(filename, rel string) ([]Document, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	var headers []string
	var documents []Document
	rowNumber := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		rowNumber++
		if rowNumber == 1 {
			headers = row
			if len(headers) > 0 {
				headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
			}
			continue
		}
		if doc, ok := tabularDocument(rel, "", rowNumber, headers, row); ok {
			documents = append(documents, doc)
		}
	}
	return documents, nil
}

type xlsxWorkbook struct {
	Sheets []struct {
		Name string `xml:"name,attr"`
		ID   string `xml:"id,attr"`
	} `xml:"sheets>sheet"`
}

type xlsxRelationships struct {
	Items []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

type xlsxSheet struct {
	Rows []struct {
		Number int `xml:"r,attr"`
		Cells  []struct {
			Ref    string `xml:"r,attr"`
			Type   string `xml:"t,attr"`
			Value  string `xml:"v"`
			Inline struct {
				Text string `xml:"t"`
				Runs []struct {
					Text string `xml:"t"`
				} `xml:"r"`
			} `xml:"is"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

type xlsxSharedStrings struct {
	Items []struct {
		Text string `xml:"t"`
		Runs []struct {
			Text string `xml:"t"`
		} `xml:"r"`
	} `xml:"si"`
}

func readXLSX(filename, rel string) ([]Document, error) {
	archive, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	files := archive.File
	workbookBytes, err := readZipFile(files, "xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	var workbook xlsxWorkbook
	if err := xml.Unmarshal(workbookBytes, &workbook); err != nil {
		return nil, fmt.Errorf("parse workbook: %w", err)
	}
	relationsBytes, err := readZipFile(files, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, err
	}
	var relations xlsxRelationships
	if err := xml.Unmarshal(relationsBytes, &relations); err != nil {
		return nil, fmt.Errorf("parse workbook relationships: %w", err)
	}
	targets := map[string]string{}
	for _, relation := range relations.Items {
		target := strings.TrimPrefix(relation.Target, "/")
		if !strings.HasPrefix(target, "xl/") {
			target = path.Join("xl", target)
		}
		if !strings.HasPrefix(target, "xl/") || strings.Contains(target, "../") {
			return nil, fmt.Errorf("invalid XLSX sheet path %q", relation.Target)
		}
		targets[relation.ID] = target
	}
	var shared []string
	if zipFile(files, "xl/sharedStrings.xml") != nil {
		sharedBytes, err := readZipFile(files, "xl/sharedStrings.xml")
		if err != nil {
			return nil, err
		}
		var stringsTable xlsxSharedStrings
		if err := xml.Unmarshal(sharedBytes, &stringsTable); err != nil {
			return nil, err
		}
		for _, item := range stringsTable.Items {
			value := item.Text
			for _, run := range item.Runs {
				value += run.Text
			}
			shared = append(shared, value)
		}
	}
	var documents []Document
	for _, entry := range workbook.Sheets {
		sheetPath := targets[entry.ID]
		if sheetPath == "" {
			return nil, fmt.Errorf("sheet %q has no relationship", entry.Name)
		}
		sheetBytes, err := readZipFile(files, sheetPath)
		if err != nil {
			return nil, err
		}
		var sheet xlsxSheet
		if err := xml.Unmarshal(sheetBytes, &sheet); err != nil {
			return nil, fmt.Errorf("parse sheet %q: %w", entry.Name, err)
		}
		var headers []string
		for index, row := range sheet.Rows {
			values := []string{}
			for _, cell := range row.Cells {
				column := xlsxColumnIndex(cell.Ref)
				if column < 0 {
					return nil, fmt.Errorf("invalid cell reference %q", cell.Ref)
				}
				if column > 16383 {
					return nil, fmt.Errorf("cell column exceeds XLSX limit")
				}
				for len(values) <= column {
					values = append(values, "")
				}
				value := cell.Value
				switch cell.Type {
				case "s":
					sharedIndex, err := strconv.Atoi(value)
					if err != nil || sharedIndex < 0 || sharedIndex >= len(shared) {
						return nil, fmt.Errorf("invalid shared string index %q", value)
					}
					value = shared[sharedIndex]
				case "inlineStr":
					value = cell.Inline.Text
					for _, run := range cell.Inline.Runs {
						value += run.Text
					}
				}
				values[column] = value
			}
			if index == 0 {
				headers = values
				continue
			}
			rowNumber := row.Number
			if rowNumber == 0 {
				rowNumber = index + 1
			}
			if doc, ok := tabularDocument(rel, entry.Name, rowNumber, headers, values); ok {
				documents = append(documents, doc)
			}
		}
	}
	return documents, nil
}

func xlsxColumnIndex(reference string) int {
	index := 0
	for _, char := range reference {
		if char < 'A' || char > 'Z' {
			break
		}
		index = index*26 + int(char-'A'+1)
	}
	return index - 1
}

func tabularDocument(rel, sheet string, rowNumber int, headers, values []string) (Document, bool) {
	var fields []string
	for index, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		name := fmt.Sprintf("Kolom %d", index+1)
		if index < len(headers) && strings.TrimSpace(headers[index]) != "" {
			name = strings.TrimSpace(headers[index])
		}
		fields = append(fields, name+": "+value)
	}
	if len(fields) == 0 {
		return Document{}, false
	}
	header := "Dokumen: " + rel + "\n"
	metadata := map[string]any{"path": rel, "row": rowNumber}
	field := fmt.Sprintf("row:%d", rowNumber)
	if sheet != "" {
		header += "Sheet: " + sheet + "\n"
		metadata["sheet"] = sheet
		field = "sheet:" + sheet + "/" + field
	}
	header += fmt.Sprintf("Baris: %d\n\n", rowNumber)
	return Document{PageContent: strings.Join(fields, "\n"), Header: header, Field: field, Metadata: metadata}, true
}

func zipFile(files []*zip.File, name string) *zip.File {
	for _, file := range files {
		if file.Name == name {
			return file
		}
	}
	return nil
}

func readZipFile(files []*zip.File, name string) ([]byte, error) {
	file := zipFile(files, name)
	if file == nil {
		return nil, fmt.Errorf("archive has no %s", name)
	}
	stream, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	return io.ReadAll(stream)
}
