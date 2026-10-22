package specextract

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extractXMind(sourcePath, imagesDir string) ([]string, error) {
	zr, err := zip.OpenReader(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("open xmind: %w", err)
	}
	defer zr.Close()

	// Find content.json
	var contentFile *zip.File
	for _, f := range zr.File {
		if f.Name == "content.json" {
			contentFile = f
			break
		}
	}
	if contentFile == nil {
		return nil, fmt.Errorf("xmind: content.json not found (XML format not supported)")
	}

	// Read content.json
	rc, err := contentFile.Open()
	if err != nil {
		return nil, fmt.Errorf("xmind: open content.json: %w", err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, fmt.Errorf("xmind: read content.json: %w", err)
	}

	// Extract embedded images from resources/ or attachments/
	imgCount := 0
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "resources/") && !strings.HasPrefix(f.Name, "attachments/") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" && ext != ".bmp" && ext != ".svg" {
			continue
		}
		imgCount++
		imgFilename := fmt.Sprintf("xmind-img%d%s", imgCount, ext)
		imgPath := filepath.Join(imagesDir, imgFilename)
		if err := extractZipFile(f, imgPath); err != nil {
			continue // skip failed images
		}
	}

	// Parse JSON — can be array (multiple sheets) or object
	var sheets []xmindSheet
	if err := json.Unmarshal(data, &sheets); err != nil {
		// Try single object
		var single xmindSheet
		if err2 := json.Unmarshal(data, &single); err2 != nil {
			return nil, fmt.Errorf("xmind: parse content.json: %w", err)
		}
		sheets = []xmindSheet{single}
	}

	var lines []string
	for _, sheet := range sheets {
		root := sheet.RootTopic
		if root.Title == "" {
			// Sheet itself has a title — treat it as root topic
			root = xmindTopic{Title: sheet.Title}
		}
		walkTopic(&lines, root, 0)
	}

	return lines, nil
}

type xmindSheet struct {
	RootTopic xmindTopic `json:"rootTopic"`
	Title     string     `json:"title"`
}

type xmindTopic struct {
	Title    string          `json:"title"`
	Notes    json.RawMessage `json:"notes,omitempty"`
	Children json.RawMessage `json:"children,omitempty"`
}

type xmindNotes struct {
	Plain struct {
		Content string `json:"content"`
	} `json:"plain"`
}

type xmindChildrenObj struct {
	Attached []xmindTopic `json:"attached"`
}

func walkTopic(lines *[]string, topic xmindTopic, depth int) {
	title := strings.TrimSpace(topic.Title)
	if title == "" {
		return
	}

	indent := strings.Repeat("  ", depth)
	if depth == 0 {
		*lines = append(*lines, fmt.Sprintf("\n## Topic: %s", title))
	} else {
		*lines = append(*lines, fmt.Sprintf("%s- %s", indent, title))
	}

	// Notes
	if len(topic.Notes) > 0 {
		var notes xmindNotes
		if json.Unmarshal(topic.Notes, &notes) == nil && strings.TrimSpace(notes.Plain.Content) != "" {
			for _, line := range strings.Split(strings.TrimSpace(notes.Plain.Content), "\n") {
				*lines = append(*lines, fmt.Sprintf("%s  > %s", indent, line))
			}
		}
	}

	// Children — can be object with "attached" or array
	if len(topic.Children) == 0 {
		return
	}
	var childrenObj xmindChildrenObj
	if json.Unmarshal(topic.Children, &childrenObj) == nil {
		for _, child := range childrenObj.Attached {
			walkTopic(lines, child, depth+1)
		}
		return
	}
	var childrenArr []xmindTopic
	if json.Unmarshal(topic.Children, &childrenArr) == nil {
		for _, child := range childrenArr {
			walkTopic(lines, child, depth+1)
		}
	}
}

func extractZipFile(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}
