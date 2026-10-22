package specextract

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func extractPPTX(sourcePath, imagesDir string) ([]string, error) {
	zr, err := zip.OpenReader(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("open pptx: %w", err)
	}
	defer zr.Close()

	// Collect slide files sorted by number
	var slideFiles []string
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			slideFiles = append(slideFiles, f.Name)
		}
	}
	sort.Strings(slideFiles)

	var lines []string

	for slideIdx, slidePath := range slideFiles {
		slideNum := slideIdx + 1

		// Parse slide relationships for images
		relsPath := strings.Replace(slidePath, "ppt/slides/", "ppt/slides/_rels/", 1) + ".rels"
		slideRels := parsePPTXRels(zr, relsPath)

		// Parse slide XML
		slideData, err := readZipFile(zr, slidePath)
		if err != nil {
			continue
		}

		var slide pptxSlide
		if err := xml.Unmarshal(slideData, &slide); err != nil {
			continue
		}

		// Find title
		title := ""
		for _, sp := range slide.CSld.SpTree.Shapes {
			if sp.NvSpPr.NvPr.Ph.Type == "title" || sp.NvSpPr.NvPr.Ph.Type == "ctrTitle" {
				title = sp.Text()
				break
			}
		}

		header := fmt.Sprintf("Slide %d", slideNum)
		if title != "" {
			header += ": " + title
		}
		lines = append(lines, fmt.Sprintf("\n## %s", header))

		imgIdx := 0

		// Process shapes
		for _, sp := range slide.CSld.SpTree.Shapes {
			text := sp.Text()
			if text != "" && text != title {
				lines = append(lines, "- "+text)
			}
		}

		// Process pictures
		for _, pic := range slide.CSld.SpTree.Pics {
			embedID := pic.BlipFill.Blip.Embed
			if embedID == "" {
				continue
			}
			target, ok := slideRels[embedID]
			if !ok {
				continue
			}
			// target is relative like "../media/image1.png"
			mediaPath := resolveMediaPath(target)
			for _, f := range zr.File {
				if f.Name == mediaPath {
					imgIdx++
					ext := filepath.Ext(f.Name)
					imgFilename := fmt.Sprintf("slide%d-img%d%s", slideNum, imgIdx, ext)
					imgPath := filepath.Join(imagesDir, imgFilename)
					if extractZipFile(f, imgPath) == nil {
						lines = append(lines, fmt.Sprintf("[image: images/%s]", imgFilename))
					}
					break
				}
			}
		}

		// Process tables in group shapes
		for _, grp := range slide.CSld.SpTree.GraphicFrames {
			tbl := grp.Graphic.GraphicData.Table
			if tbl == nil {
				continue
			}
			for _, row := range tbl.Rows {
				var cells []string
				for _, cell := range row.Cells {
					cells = append(cells, cell.Text())
				}
				if hasNonEmpty(cells) {
					lines = append(lines, strings.Join(cells, " | "))
				}
			}
		}

		// Slide notes
		notesPath := strings.Replace(slidePath, "ppt/slides/slide", "ppt/notesSlides/notesSlide", 1)
		if notesData, err := readZipFile(zr, notesPath); err == nil {
			var notes pptxNotes
			if xml.Unmarshal(notesData, &notes) == nil {
				text := notes.Text()
				if text != "" {
					lines = append(lines, fmt.Sprintf("<!-- Notes: %s -->", text))
				}
			}
		}
	}

	return lines, nil
}

// --- OOXML structs for pptx ---

type pptxSlide struct {
	XMLName xml.Name `xml:"sld"`
	CSld    pptxCSld `xml:"cSld"`
}

type pptxCSld struct {
	SpTree pptxSpTree `xml:"spTree"`
}

type pptxSpTree struct {
	Shapes        []pptxShape        `xml:"sp"`
	Pics          []pptxPic          `xml:"pic"`
	GraphicFrames []pptxGraphicFrame `xml:"graphicFrame"`
}

type pptxShape struct {
	NvSpPr pptxNvSpPr `xml:"nvSpPr"`
	TxBody pptxTxBody `xml:"txBody"`
}

func (s *pptxShape) Text() string {
	var parts []string
	for _, p := range s.TxBody.Paragraphs {
		text := p.Text()
		if text != "" {
			indent := ""
			if p.PPr.Lvl > 0 {
				indent = strings.Repeat("  ", p.PPr.Lvl)
			}
			parts = append(parts, indent+text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

type pptxNvSpPr struct {
	NvPr pptxNvPr `xml:"nvPr"`
}

type pptxNvPr struct {
	Ph pptxPh `xml:"ph"`
}

type pptxPh struct {
	Type string `xml:"type,attr"`
}

type pptxTxBody struct {
	Paragraphs []pptxParagraph `xml:"p"`
}

type pptxParagraph struct {
	PPr  pptxPPr    `xml:"pPr"`
	Runs []pptxRun  `xml:"r"`
}

type pptxPPr struct {
	Lvl int `xml:"lvl,attr"`
}

type pptxRun struct {
	Text string `xml:"t"`
}

func (p *pptxParagraph) Text() string {
	var parts []string
	for _, r := range p.Runs {
		if r.Text != "" {
			parts = append(parts, r.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

type pptxPic struct {
	BlipFill pptxBlipFill `xml:"blipFill"`
}

type pptxBlipFill struct {
	Blip pptxBlip `xml:"blip"`
}

type pptxBlip struct {
	Embed string `xml:"embed,attr"`
}

type pptxGraphicFrame struct {
	Graphic pptxGraphic `xml:"graphic"`
}

type pptxGraphic struct {
	GraphicData pptxGraphicData `xml:"graphicData"`
}

type pptxGraphicData struct {
	Table *pptxTable `xml:"tbl"`
}

type pptxTable struct {
	Rows []pptxTblRow `xml:"tr"`
}

type pptxTblRow struct {
	Cells []pptxTblCell `xml:"tc"`
}

type pptxTblCell struct {
	TxBody pptxTxBody `xml:"txBody"`
}

func (c *pptxTblCell) Text() string {
	var parts []string
	for _, p := range c.TxBody.Paragraphs {
		t := p.Text()
		if t != "" {
			parts = append(parts, t)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// Notes slide
type pptxNotes struct {
	CSld pptxCSld `xml:"cSld"`
}

func (n *pptxNotes) Text() string {
	var parts []string
	for _, sp := range n.CSld.SpTree.Shapes {
		t := sp.Text()
		if t != "" {
			parts = append(parts, t)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// --- Helpers ---

func parsePPTXRels(zr *zip.ReadCloser, path string) map[string]string {
	result := map[string]string{}
	data, err := readZipFile(zr, path)
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
	return result
}

func readZipFile(zr *zip.ReadCloser, path string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == path {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("file not found in zip: %s", path)
}

func resolveMediaPath(relTarget string) string {
	// Relative targets like "../media/image1.png" → "ppt/media/image1.png"
	relTarget = strings.TrimPrefix(relTarget, "../")
	if !strings.HasPrefix(relTarget, "ppt/") {
		relTarget = "ppt/" + relTarget
	}
	return relTarget
}

func writeImageFile(data []byte, path string) error {
	return os.WriteFile(path, data, 0o644)
}
