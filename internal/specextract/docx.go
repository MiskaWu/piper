package specextract

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

func extractDOCX(sourcePath, imagesDir string) ([]string, error) {
	zr, err := zip.OpenReader(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("open docx: %w", err)
	}
	defer zr.Close()

	// Extract images from word/media/
	imgCount := 0
	imageRIDs := map[string]string{} // rId → filename

	// Parse relationships to map rIds to media files
	rels := parseRels(zr, "word/_rels/document.xml.rels")

	for rID, target := range rels {
		if !strings.Contains(target, "media/") && !strings.Contains(target, "image") {
			continue
		}
		// Find the actual file in zip
		mediaPath := target
		if !strings.HasPrefix(mediaPath, "word/") {
			mediaPath = "word/" + mediaPath
		}
		for _, f := range zr.File {
			if f.Name == mediaPath {
				imgCount++
				ext := filepath.Ext(f.Name)
				imgFilename := fmt.Sprintf("doc-img%d%s", imgCount, ext)
				imgPath := filepath.Join(imagesDir, imgFilename)
				if err := extractZipFile(f, imgPath); err == nil {
					imageRIDs[rID] = imgFilename
				}
				break
			}
		}
	}

	// Parse document.xml
	var docFile *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return nil, fmt.Errorf("docx: word/document.xml not found")
	}

	rc, err := docFile.Open()
	if err != nil {
		return nil, fmt.Errorf("docx: open document.xml: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("docx: read document.xml: %w", err)
	}

	var doc docxDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("docx: parse document.xml: %w", err)
	}

	var lines []string

	for _, item := range doc.Body.Items {
		switch {
		case item.Paragraph != nil:
			p := item.Paragraph
			text := p.Text()
			embedIDs := p.EmbedIDs()

			if text == "" && len(embedIDs) == 0 {
				continue
			}

			// Heading detection
			if p.Properties.Style.Val != "" && strings.HasPrefix(p.Properties.Style.Val, "Heading") {
				level := 2
				if len(p.Properties.Style.Val) > 7 {
					l := p.Properties.Style.Val[7:]
					if l >= "1" && l <= "6" {
						level = int(l[0]-'0') + 1
						if level > 6 {
							level = 6
						}
					}
				}
				lines = append(lines, fmt.Sprintf("\n%s %s", strings.Repeat("#", level), text))
			} else if text != "" {
				lines = append(lines, text)
			}

			for _, rid := range embedIDs {
				if fn, ok := imageRIDs[rid]; ok {
					lines = append(lines, fmt.Sprintf("[image: images/%s]", fn))
				}
			}

		case item.Table != nil:
			lines = append(lines, "")
			for _, row := range item.Table.Rows {
				var cells []string
				for _, cell := range row.Cells {
					cells = append(cells, cell.Text())
				}
				if hasNonEmpty(cells) {
					lines = append(lines, strings.Join(cells, " | "))
				}
			}
		}
	}

	return lines, nil
}

// --- OOXML structs for docx ---

type docxDocument struct {
	XMLName xml.Name `xml:"document"`
	Body    docxBody `xml:"body"`
}

type docxBody struct {
	Items []docxBodyItem `xml:",any"`
}

type docxBodyItem struct {
	Paragraph *docxParagraph `xml:"p"`
	Table     *docxTable     `xml:"tbl"`
}

func (b *docxBody) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		item := docxBodyItem{}
		switch se.Name.Local {
		case "p":
			var p docxParagraph
			if err := d.DecodeElement(&p, &se); err != nil {
				return err
			}
			item.Paragraph = &p
		case "tbl":
			var t docxTable
			if err := d.DecodeElement(&t, &se); err != nil {
				return err
			}
			item.Table = &t
		default:
			if err := d.Skip(); err != nil {
				return err
			}
			continue
		}
		b.Items = append(b.Items, item)
	}
}

type docxParagraph struct {
	Properties docxParagraphProps `xml:"pPr"`
	Runs       []docxRun          `xml:"r"`
}

func (p *docxParagraph) Text() string {
	var parts []string
	for _, r := range p.Runs {
		if r.Text.Value != "" {
			parts = append(parts, r.Text.Value)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func (p *docxParagraph) EmbedIDs() []string {
	var ids []string
	for _, r := range p.Runs {
		for _, d := range r.Drawings {
			for _, blip := range d.Blips {
				if blip.Embed != "" {
					ids = append(ids, blip.Embed)
				}
			}
		}
	}
	return ids
}

type docxParagraphProps struct {
	Style docxStyle `xml:"pStyle"`
}

type docxStyle struct {
	Val string `xml:"val,attr"`
}

type docxRun struct {
	Text     docxText      `xml:"t"`
	Drawings []docxDrawing `xml:"drawing"`
}

type docxText struct {
	Value string `xml:",chardata"`
}

type docxDrawing struct {
	Blips []docxBlip `xml:"inline>graphic>graphicData>pic>blipFill>blip"`
}

// UnmarshalXML handles the nested namespace-heavy drawing structure.
func (d *docxDrawing) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	// Walk the XML tree looking for blip elements
	depth := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "blip" {
				var blip docxBlip
				for _, attr := range t.Attr {
					if attr.Name.Local == "embed" {
						blip.Embed = attr.Value
					}
				}
				if blip.Embed != "" {
					d.Blips = append(d.Blips, blip)
				}
			}
		case xml.EndElement:
			if depth == 0 {
				return nil
			}
			depth--
		}
	}
}

type docxBlip struct {
	Embed string
}

type docxTable struct {
	Rows []docxRow `xml:"tr"`
}

type docxRow struct {
	Cells []docxCell `xml:"tc"`
}

type docxCell struct {
	Paragraphs []docxParagraph `xml:"p"`
}

func (c *docxCell) Text() string {
	var parts []string
	for _, p := range c.Paragraphs {
		t := p.Text()
		if t != "" {
			parts = append(parts, t)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// --- Relationship parsing ---

type relsXML struct {
	Rels []relXML `xml:"Relationship"`
}

type relXML struct {
	ID     string `xml:"Id,attr"`
	Target string `xml:"Target,attr"`
	Type   string `xml:"Type,attr"`
}

func parseRels(zr *zip.ReadCloser, path string) map[string]string {
	result := map[string]string{}
	for _, f := range zr.File {
		if f.Name != path {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return result
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return result
		}
		var rels relsXML
		if xml.Unmarshal(data, &rels) != nil {
			return result
		}
		for _, r := range rels.Rels {
			if strings.Contains(r.Type, "image") {
				result[r.ID] = r.Target
			}
		}
		break
	}
	return result
}

func hasNonEmpty(cells []string) bool {
	for _, c := range cells {
		if strings.TrimSpace(c) != "" {
			return true
		}
	}
	return false
}
