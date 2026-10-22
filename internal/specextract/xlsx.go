package specextract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

func extractXLSX(sourcePath, imagesDir string) ([]string, error) {
	f, err := excelize.OpenFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	var lines []string

	for _, sheetName := range f.GetSheetList() {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			return nil, fmt.Errorf("read sheet %s: %w", sheetName, err)
		}

		// Skip empty sheets
		hasContent := false
		for _, row := range rows {
			for _, cell := range row {
				if strings.TrimSpace(cell) != "" {
					hasContent = true
					break
				}
			}
			if hasContent {
				break
			}
		}
		if !hasContent {
			continue
		}

		lines = append(lines, fmt.Sprintf("\n## Sheet: %s", sheetName))

		// Extract images for this sheet
		imageMap := map[string][]string{} // cell ref → []image filenames
		picCells, err := f.GetPictureCells(sheetName)
		if err == nil {
			for _, cell := range picCells {
				pics, err := f.GetPictures(sheetName, cell)
				if err != nil || len(pics) == 0 {
					continue
				}
				for i, pic := range pics {
					safeSheet := strings.ReplaceAll(strings.ReplaceAll(sheetName, "/", "-"), " ", "_")
					imgFilename := fmt.Sprintf("%s-%s-img%d%s", safeSheet, cell, i+1, pic.Extension)
					imgPath := filepath.Join(imagesDir, imgFilename)
					if err := os.WriteFile(imgPath, pic.File, 0o644); err != nil {
						continue
					}
					imageMap[cell] = append(imageMap[cell], imgFilename)
				}
			}
		}

		// Output rows
		for rowIdx, row := range rows {
			cells := make([]string, len(row))
			for i, cell := range row {
				cells[i] = strings.TrimSpace(cell)
			}

			// Skip empty rows
			nonEmpty := 0
			for _, c := range cells {
				if c != "" {
					nonEmpty++
				}
			}
			if nonEmpty == 0 {
				continue
			}

			rowNum := rowIdx + 1
			if nonEmpty > 1 {
				lines = append(lines, fmt.Sprintf("<!-- Row %d --> %s", rowNum, strings.Join(cells, " | ")))
			} else {
				// Single-value row: only join non-empty cells
				var vals []string
				for _, c := range cells {
					if c != "" {
						vals = append(vals, c)
					}
				}
				lines = append(lines, fmt.Sprintf("<!-- Row %d --> %s", rowNum, strings.Join(vals, " | ")))
			}

			// Insert images anchored to cells in this row
			for col := 0; col < len(row); col++ {
				cellRef, _ := excelize.CoordinatesToCellName(col+1, rowNum)
				if imgs, ok := imageMap[cellRef]; ok {
					for _, img := range imgs {
						lines = append(lines, fmt.Sprintf("[image: images/%s]", img))
					}
				}
			}
		}
	}

	return lines, nil
}
