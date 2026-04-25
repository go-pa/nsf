package nsf

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRegenerateStubs is a utility test that regenerates NSF/NSFE stubs from full files.
// It only runs if testdata/nsf-src exists.
// It strips all program data (ROM) from the files to avoid copyright issues while preserving metadata.
func TestRegenerateStubs(t *testing.T) {
	if os.Getenv("TEST_REGENERATE_NSF_TESTDATA") == "" {
		t.Skip("set TEST_REGENERATE_NSF_TESTDATA to regenerate test data")
	}
	srcDir := filepath.Join("testdata", "nsf-src")
	dstDir := filepath.Join("testdata", "nsf")

	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		t.Skipf("Source directory %s does not exist, skipping stub regeneration", srcDir)
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("Failed to create destination directory %s: %v", dstDir, err)
	}

	var nsfFiles, nsfeFiles []string
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".nsf" {
			nsfFiles = append(nsfFiles, path)
		} else if ext == ".nsfe" {
			nsfeFiles = append(nsfeFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to walk source directory %s: %v", srcDir, err)
	}

	rand.Shuffle(len(nsfFiles), func(i, j int) {
		nsfFiles[i], nsfFiles[j] = nsfFiles[j], nsfFiles[i]
	})
	rand.Shuffle(len(nsfeFiles), func(i, j int) {
		nsfeFiles[i], nsfeFiles[j] = nsfeFiles[j], nsfeFiles[i]
	})

	const maxEntries = 200
	var selected []string
	half := maxEntries / 2

	// Take up to half from each
	nNSF := min(len(nsfFiles), half)
	nNSFE := min(len(nsfeFiles), half)

	// If one is short, try to take more from the other to reach maxEntries
	if nNSF < half {
		nNSFE = min(len(nsfeFiles), maxEntries-nNSF)
	} else if nNSFE < half {
		nNSF = min(len(nsfFiles), maxEntries-nNSFE)
	}

	selected = append(selected, nsfFiles[:nNSF]...)
	selected = append(selected, nsfeFiles[:nNSFE]...)

	for _, srcPath := range selected {
		dstPath := filepath.Join(dstDir, filepath.Base(srcPath))

		data, err := os.ReadFile(srcPath)
		if err != nil {
			t.Errorf("Failed to read %s: %v", srcPath, err)
			continue
		}

		if len(data) < 4 {
			t.Errorf("File %s is too small", srcPath)
			continue
		}

		stub := make([]byte, len(data))
		copy(stub, data)

		magic := data[:4]
		if bytes.Equal(magic, []byte("NESM")) {
			stripNSF(stub)
		} else if bytes.Equal(magic, []byte("NSFE")) {
			stripNSFE(stub)
		} else {
			t.Errorf("Unknown format for %s", srcPath)
			continue
		}

		err = os.WriteFile(dstPath, stub, 0o644)
		if err != nil {
			t.Errorf("Failed to write %s: %v", dstPath, err)
		} else {
			t.Logf("Regenerated stub: %s", dstPath)
		}
	}
}

func stripNSF(data []byte) {
	if len(data) < 128 {
		return
	}
	version := data[5]
	dataLen := 0
	if version >= 2 {
		dataLen = int(uint32(data[125]) | uint32(data[126])<<8 | uint32(data[127])<<16)
	}

	if dataLen > 0 && 128+dataLen <= len(data) {
		// Zero out only the ROM data, keep NSF2 chunks
		for i := 128; i < 128+dataLen; i++ {
			data[i] = 0
		}
	} else {
		// Zero out everything after header
		for i := 128; i < len(data); i++ {
			data[i] = 0
		}
	}
}

func stripNSFE(data []byte) {
	pos := 4
	for pos+8 <= len(data) {
		length := binary.LittleEndian.Uint32(data[pos : pos+4])
		chunkType := string(data[pos+4 : pos+8])
		nextPos := pos + 8 + int(length)
		if chunkType == "DATA" {
			start := pos + 8
			end := start + int(length)
			if end > len(data) {
				end = len(data)
			}
			for i := start; i < end; i++ {
				data[i] = 0
			}
		}
		if nextPos <= pos { // Avoid infinite loop or weirdness
			break
		}
		pos = nextPos
		if chunkType == "NEND" {
			break
		}
	}
}
