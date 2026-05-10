package nsf

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type TestFile struct {
	Name string
	Data []byte
}

type TestFiles []TestFile

func (files TestFiles) Get(t *testing.T, name string) TestFile {
	t.Helper()
	for _, f := range files {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("file not found in archive: %s", name)
	return TestFile{}
}

var nfsFiles TestFiles
var once sync.Once

func getFiles() TestFiles {
	once.Do(func() {
		data, err := readTarGz("testdata/nsf.tar.gz")
		if err != nil {
			panic(err)
		}
		nfsFiles = data
	})
	return nfsFiles
}

func readTarGz(path string) (TestFiles, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	var result TestFiles
	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		result = append(result, TestFile{Name: hdr.Name, Data: data})
	}
	return result, nil
}

func TestParseDir(t *testing.T) {
	dir := os.Getenv("TEST_PARSE_NSF_DIR")
	if dir == "" {
		t.Skip("set TEST_PARSE_NSF_DIR to run")
	}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, "._") || strings.Contains(path, "__MACOSX") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".nsf" && ext != ".nsfe" {
			return nil
		}
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open(path)
			if err != nil {
				t.Fatalf("failed to open file: %v", err)
			}
			defer f.Close()
			m, err := Parse(f)
			if err != nil {
				t.Errorf("failed to parse: %v", err)
				return
			}
			if ext == ".nsf" && m.Format != FormatNSF {
				t.Errorf("expected format NSF, got %s", m.Format)
			}
			if ext == ".nsfe" && m.Format != FormatNSFE {
				t.Errorf("expected format NSFE, got %s", m.Format)
			}
			if m.TotalSongs <= 0 {
				t.Errorf("total songs should be > 0, got %d", m.TotalSongs)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk failed: %v", err)
	}
}

func TestParse(t *testing.T) {
	for _, f := range getFiles() {
		base := filepath.Base(f.Name)
		if strings.HasPrefix(base, "._") || strings.Contains(f.Name, "__MACOSX") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext != ".nsf" && ext != ".nsfe" {
			continue
		}
		f := f
		t.Run(f.Name, func(t *testing.T) {
			t.Parallel()
			m, err := ParseBytes(f.Data)
			if err != nil {
				t.Errorf("failed to parse: %v", err)
				return
			}
			if ext == ".nsf" && m.Format != FormatNSF {
				t.Errorf("expected format NSF, got %s", m.Format)
			}
			if ext == ".nsfe" && m.Format != FormatNSFE {
				t.Errorf("expected format NSFE, got %s", m.Format)
			}
			if m.TotalSongs <= 0 {
				t.Errorf("total songs should be > 0, got %d", m.TotalSongs)
			}
		})
	}
}

func TestDecodeString(t *testing.T) {
	tests := []struct {
		input    []byte
		expected string
	}{
		{[]byte("Hello"), "Hello"},
		{[]byte("Hëllö"), "Hëllö"},                      // Valid UTF-8
		{[]byte{0x48, 0xEB, 0x6C, 0x6C, 0xF6}, "Hëllö"}, // Windows-1252
		{[]byte{0x80}, "€"},                             // Windows-1252: 0x80 is €
		{[]byte{0x01, 0x41, 0x1F, 0x42}, "AB"},          // Control characters 0x01 and 0x1F should be stripped
	}
	for _, tt := range tests {
		got := decodeString(tt.input)
		if got != tt.expected {
			t.Errorf("decodeString(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseNSFString(t *testing.T) {
	tests := []struct {
		input    []byte
		expected string
	}{
		{[]byte("Title\x00Garbage"), "Title"},
		{[]byte{0x00, 0x00, 0x10}, ""},
		{append([]byte("VRC 6: Attack of the DVDs"), 0, 0, 0, 0, 0, 0x18), "VRC 6: Attack of the DVDs"},
	}
	for _, tt := range tests {
		got := parseNSFString(tt.input)
		if got != tt.expected {
			t.Errorf("parseNSFString(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestTracks(t *testing.T) {
	f := getFiles().Get(t, "nsf/Aa Yakyuu Jinsei Icchokusen.nsfe")
	m, err := ParseBytes(f.Data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if m.Format != FormatNSFE {
		t.Errorf("expected format NSFE, got %s", m.Format)
	}

	if len(m.Tracks) == 0 {
		t.Errorf("expected some tracks, got 0")
	}

	// Check if at least one track has info (this file should have it)
	hasInfo := false
	tracks := m.PlaylistTracks()

	if len(tracks) == 0 {
		if len(m.Playlist) > 0 {
			t.Errorf("expected tracks since playlist is present")
		}
		// For this specific test file, we expect tracks.
		// If it's empty, it's an error for this test case.
		t.Errorf("expected some tracks for this NSFE file, got 0")
	}

	if len(m.Playlist) == 0 {
		for i, tr := range tracks {
			if tr.Number != i+1 {
				t.Errorf("expected track %d number to be %d, got %d", i, i+1, tr.Number)
			}
		}
	} else {
		seen := make(map[int]bool)
		for _, idx := range m.Playlist {
			seen[idx] = true
		}
		expected := len(seen)
		if len(tracks) != expected {
			t.Errorf("expected %d tracks (unique playlist entries), got %d", expected, len(tracks))
		}
	}

	for _, tr := range tracks {
		if tr.Label != "" || tr.Time > 0 || tr.Fade > 0 {
			hasInfo = true
		}
	}
	if !hasInfo {
		t.Errorf("expected at least one track to have metadata info")
	}
}

func TestSFXTracks(t *testing.T) {
	f := getFiles().Get(t, "nsf/Athletic World (NTSC, All Songs + SFX).nsfe")
	m, err := ParseBytes(f.Data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if m.Format != FormatNSFE {
		t.Errorf("expected format NSFE, got %s", m.Format)
	}

	sfxTracks := m.SFXPlaylistTracks()
	if len(m.SFXPlaylist) == 0 {
		t.Skip("skipping SFX tracks checks: file does not have an SFX playlist")
	}

	if len(sfxTracks) == 0 {
		t.Errorf("expected some SFX tracks, got 0")
	}

	seen := make(map[int]bool)
	for _, idx := range m.SFXPlaylist {
		seen[idx] = true
	}
	expected := len(seen)
	if len(sfxTracks) != expected {
		t.Errorf("expected %d SFX tracks (unique playlist entries), got %d", expected, len(sfxTracks))
	}
}

func TestTrack_IsZero(t *testing.T) {
	if !(Track{}).IsZero() {
		t.Error("expected Track{} to be zero")
	}
	if (Track{Number: 1}).IsZero() {
		t.Error("expected Track{Number: 1} to not be zero")
	}
}

func TestParse_Errors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		err  error
	}{
		{"Empty", []byte{}, ErrTruncated},
		{"ShortMagic", []byte("NES"), ErrTruncated},
		{"InvalidMagic", []byte("BOGU"), ErrInvalidMagic},
		{"NSFInvalidHeader", []byte("NESM\x00"), ErrTruncated},
		{"NSFInvalidSubMagic", append([]byte("NESM"), make([]byte, 124)...), ErrInvalidMagic},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBytes(tt.data)
			if err != tt.err {
				t.Errorf("ParseBytes() error = %v, want %v", err, tt.err)
			}
			_, err = Parse(bytes.NewReader(tt.data))
			if err != tt.err {
				t.Errorf("Parse() error = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestParseCommonChunk(t *testing.T) {
	t.Run("auth", func(t *testing.T) {
		m := &Metadata{}
		m.parseCommonChunk("auth", []byte("Title\x00Artist\x00Copyright\x00Ripper"))
		if m.Title != "Title" || m.Artist != "Artist" || m.Copyright != "Copyright" || m.Ripper != "Ripper" {
			t.Errorf("got %+v", m)
		}
	})

	t.Run("plst", func(t *testing.T) {
		m := &Metadata{}
		m.parseCommonChunk("plst", []byte{0, 2, 1})
		if !reflect.DeepEqual(m.Playlist, []int{0, 2, 1}) {
			t.Errorf("got %v", m.Playlist)
		}
	})

	t.Run("psfx", func(t *testing.T) {
		m := &Metadata{}
		m.parseCommonChunk("psfx", []byte{3, 4})
		if !reflect.DeepEqual(m.SFXPlaylist, []int{3, 4}) {
			t.Errorf("got %v", m.SFXPlaylist)
		}
	})

	t.Run("tlbl", func(t *testing.T) {
		m := &Metadata{}
		m.parseCommonChunk("tlbl", []byte("Label1\x00Label2"))
		if m.Tracks[0].Label != "Label1" || m.Tracks[1].Label != "Label2" {
			t.Errorf("got %+v", m.Tracks)
		}
	})

	t.Run("taut", func(t *testing.T) {
		m := &Metadata{}
		m.parseCommonChunk("taut", []byte("Author1\x00Author2"))
		if m.Tracks[0].Author != "Author1" || m.Tracks[1].Author != "Author2" {
			t.Errorf("got %+v", m.Tracks)
		}
	})

	t.Run("text", func(t *testing.T) {
		m := &Metadata{}
		m.parseCommonChunk("text", []byte("Message"))
		if m.Message != "Message" {
			t.Errorf("got %s", m.Message)
		}
	})

	t.Run("time", func(t *testing.T) {
		m := &Metadata{}
		data := make([]byte, 8)
		binary.LittleEndian.PutUint32(data[0:4], 1000)
		binary.LittleEndian.PutUint32(data[4:8], 2000)
		m.parseCommonChunk("time", data)
		if m.Tracks[0].Time != 1000 || m.Tracks[1].Time != 2000 {
			t.Errorf("got %+v", m.Tracks)
		}
	})

	t.Run("fade", func(t *testing.T) {
		m := &Metadata{}
		data := make([]byte, 8)
		binary.LittleEndian.PutUint32(data[0:4], 500)
		binary.LittleEndian.PutUint32(data[4:8], 600)
		m.parseCommonChunk("fade", data)
		if m.Tracks[0].Fade != 500 || m.Tracks[1].Fade != 600 {
			t.Errorf("got %+v", m.Tracks)
		}
	})

	t.Run("BANK", func(t *testing.T) {
		m := &Metadata{}
		m.parseCommonChunk("BANK", []byte{1, 2, 3, 4, 5, 6, 7, 8})
		if !bytes.Equal(m.BankValues[:], []byte{1, 2, 3, 4, 5, 6, 7, 8}) {
			t.Errorf("got %v", m.BankValues)
		}
	})

	t.Run("RATE", func(t *testing.T) {
		m := &Metadata{}
		data := make([]byte, 6)
		binary.LittleEndian.PutUint16(data[0:2], 16666)
		binary.LittleEndian.PutUint16(data[2:4], 19997)
		binary.LittleEndian.PutUint16(data[4:6], 16639)
		m.parseCommonChunk("RATE", data)
		if m.NTSCSpeed != 16666 || m.PALSpeed != 19997 || m.DendySpeed != 16639 {
			t.Errorf("got %d, %d, %d", m.NTSCSpeed, m.PALSpeed, m.DendySpeed)
		}
	})

	t.Run("NSF2", func(t *testing.T) {
		m := &Metadata{}
		m.parseCommonChunk("NSF2", []byte{0x01})
		if m.NSF2Flags != 0x01 {
			t.Errorf("got %x", m.NSF2Flags)
		}
	})

	t.Run("regn", func(t *testing.T) {
		m := &Metadata{}
		m.parseCommonChunk("regn", []byte{0x02, 0x01})
		if m.Region != RegionPAL || m.PrefRegion != RegionPAL {
			t.Errorf("got region=%d pref=%d", m.Region, m.PrefRegion)
		}
	})

	t.Run("VRC7", func(t *testing.T) {
		m := &Metadata{}
		m.parseCommonChunk("VRC7", []byte{0xDE, 0xAD, 0xBE, 0xEF})
		if !bytes.Equal(m.VRC7Data, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
			t.Errorf("got %v", m.VRC7Data)
		}
	})

	t.Run("mixe", func(t *testing.T) {
		m := &Metadata{}
		m.parseCommonChunk("mixe", []byte{1, 2, 3})
		if !bytes.Equal(m.MixerData, []byte{1, 2, 3}) {
			t.Errorf("got %v", m.MixerData)
		}
	})

	t.Run("DATA", func(t *testing.T) {
		m := &Metadata{}
		m.parseCommonChunk("DATA", []byte{0x01, 0x02})
		if !bytes.Equal(m.ROMData, []byte{0x01, 0x02}) {
			t.Errorf("got %v", m.ROMData)
		}
	})
}

func TestMetadata_PlaylistTracks(t *testing.T) {
	tests := []struct {
		name     string
		metadata *Metadata
		expected []Track
	}{
		{
			"Empty",
			&Metadata{},
			nil,
		},
		{
			"NoPlaylistWithTracks",
			&Metadata{
				TotalSongs: 2,
				Tracks: []Track{
					{Number: 1, Label: "T1"},
				},
			},
			[]Track{{Number: 1, Label: "T1"}},
		},
		{
			"WithPlaylist",
			&Metadata{
				TotalSongs: 3,
				Playlist:   []int{2, 0, 2},
				Tracks: []Track{
					{Number: 1, Label: "T1"},
					{Number: 2, Label: "T2"},
					{Number: 3, Label: "T3"},
				},
			},
			[]Track{
				{Number: 3, Label: "T3"},
				{Number: 1, Label: "T1"},
			},
		},
		{
			"PlaylistWithMissingTracks",
			&Metadata{
				TotalSongs: 3,
				Playlist:   []int{5},
				Tracks: []Track{
					{Number: 1, Label: "T1"},
				},
			},
			[]Track{
				{Number: 6},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.metadata.PlaylistTracks()
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("PlaylistTracks() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseChunks_NoNEND(t *testing.T) {
	m := &Metadata{}
	data := make([]byte, 8+4)
	binary.LittleEndian.PutUint32(data[0:4], 4)
	copy(data[4:8], "text")
	copy(data[8:12], "Test")

	err := m.parseChunks(data)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if m.Message != "Test" {
		t.Errorf("expected Message 'Test', got '%s'", m.Message)
	}
}

func TestParseNSFEChunks_NoNEND(t *testing.T) {
	m := &Metadata{}
	data := make([]byte, 8+4)
	binary.LittleEndian.PutUint32(data[0:4], 4)
	copy(data[4:8], "text")
	copy(data[8:12], "Test")

	err := m.parseNSFEChunks(data)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if m.Message != "Test" {
		t.Errorf("expected Message 'Test', got '%s'", m.Message)
	}
}

func TestParseNSFE_INFO_Short(t *testing.T) {
	// INFO shorter than the full 10 bytes must not error; only complete fields are parsed.
	m := &Metadata{}
	data := make([]byte, 8+5)
	binary.LittleEndian.PutUint32(data[0:4], 5)
	copy(data[4:8], "INFO")
	if err := m.parseNSFEChunks(data); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

type errorReader struct{}

func (e errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func TestParse_ReaderError(t *testing.T) {
	_, err := Parse(errorReader{})
	if err != io.ErrUnexpectedEOF {
		t.Errorf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}

func TestParseNSF_TruncatedROM(t *testing.T) {
	header := make([]byte, 128)
	copy(header[0:4], "NESM")
	header[4] = 0x1a
	header[5] = 2    // Version 2
	header[125] = 10 // 10 bytes of ROM data
	header[126] = 0
	header[127] = 0

	data := append(header, make([]byte, 5)...) // Only 5 bytes of ROM data
	_, err := ParseBytes(data)
	if err != ErrTruncated {
		t.Errorf("expected ErrTruncated, got %v", err)
	}
}

func TestParseChunks_Error(t *testing.T) {
	m := &Metadata{}
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], 10) // Length 10
	copy(data[4:8], "text")
	// No data follows

	err := m.parseChunks(data)
	if err != ErrTruncated {
		t.Errorf("expected ErrTruncated, got %v", err)
	}
}

func TestParseNSFEChunks_Error(t *testing.T) {
	m := &Metadata{}
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], 10) // Length 10
	copy(data[4:8], "INFO")
	// No data follows

	err := m.parseNSFEChunks(data)
	if err != ErrTruncated {
		t.Errorf("expected ErrTruncated, got %v", err)
	}
}

func TestReadChunk_Errors(t *testing.T) {
	_, _, _, err := readChunk([]byte{})
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}

	_, _, _, err = readChunk([]byte("1234567"))
	if err != ErrTruncated {
		t.Errorf("expected ErrTruncated, got %v", err)
	}

	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], 10)
	copy(data[4:8], "TEST")
	_, _, _, err = readChunk(data)
	if err != ErrTruncated {
		t.Errorf("expected ErrTruncated, got %v", err)
	}
}

func TestParseNSF_InvalidSubMagic(t *testing.T) {
	data := make([]byte, 128)
	copy(data[0:4], "NESM")
	data[4] = 0x1b // Not 0x1a
	_, err := ParseBytes(data)
	if err != ErrInvalidMagic {
		t.Errorf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestParseNSF2(t *testing.T) {
	// NESM header (124 bytes)
	header := make([]byte, 128)
	copy(header[0:4], "NESM")
	header[4] = 0x1a
	header[5] = 2                                        // Version 2
	header[6] = 1                                        // Total songs
	header[7] = 1                                        // Start song
	binary.LittleEndian.PutUint16(header[8:10], 0x8000)  // Load
	binary.LittleEndian.PutUint16(header[10:12], 0x8000) // Init
	binary.LittleEndian.PutUint16(header[12:14], 0x8003) // Play

	// Title, Artist, Copyright
	copy(header[14:46], "Title")
	copy(header[46:78], "Artist")
	copy(header[78:110], "Copyright")

	// Speed
	binary.LittleEndian.PutUint16(header[110:112], 16666) // NTSC
	binary.LittleEndian.PutUint16(header[120:122], 19997) // PAL

	header[122] = 0 // Region
	header[123] = 0 // Extra chips
	header[124] = 0 // NSF2 flags

	// NSF2DataLen (3 bytes at 125, 126, 127)
	header[125] = 4 // 4 bytes of ROM data
	header[126] = 0
	header[127] = 0

	romData := []byte{0xDE, 0xAD, 0xBE, 0xEF}

	// Chunks
	chunk1 := make([]byte, 8+4)
	binary.LittleEndian.PutUint32(chunk1[0:4], 4)
	copy(chunk1[4:8], "text")
	copy(chunk1[8:12], "Test")

	chunk2 := make([]byte, 8)
	binary.LittleEndian.PutUint32(chunk2[0:4], 0)
	copy(chunk2[4:8], "NEND")

	data := append(header, romData...)
	data = append(data, chunk1...)
	data = append(data, chunk2...)

	m, err := ParseBytes(data)
	if err != nil {
		t.Fatalf("ParseBytes failed: %v", err)
	}

	if m.Format != FormatNSF || m.Version != 2 {
		t.Errorf("expected NSF v2, got %s v%d", m.Format, m.Version)
	}
	if !bytes.Equal(m.ROMData, romData) {
		t.Errorf("expected ROM data %v, got %v", romData, m.ROMData)
	}
	if m.Message != "Test" {
		t.Errorf("expected message 'Test', got '%s'", m.Message)
	}
}

func TestParseNSFE_Truncated(t *testing.T) {
	// NSFE with no chunks is valid — the chunk loop exits immediately.
	_, err := ParseBytes([]byte("NSFE"))
	if err != nil {
		t.Errorf("expected no error for minimal NSFE, got %v", err)
	}
}

func TestParseNSFE_INFO(t *testing.T) {
	data := []byte("NSFE")

	infoData := make([]byte, 10)
	binary.LittleEndian.PutUint16(infoData[0:2], 0x8000)
	binary.LittleEndian.PutUint16(infoData[2:4], 0x8001)
	binary.LittleEndian.PutUint16(infoData[4:6], 0x8002)
	infoData[6] = 1 // Region
	infoData[7] = 2 // Extra chips
	infoData[8] = 3 // Total songs
	infoData[9] = 4 // Start song (0-indexed in INFO, so 5 in Metadata)

	chunk := make([]byte, 8+10)
	binary.LittleEndian.PutUint32(chunk[0:4], 10)
	copy(chunk[4:8], "INFO")
	copy(chunk[8:], infoData)

	data = append(data, chunk...)

	m, err := ParseBytes(data)
	if err != nil {
		t.Fatalf("ParseBytes failed: %v", err)
	}

	if m.LoadAddr != 0x8000 || m.InitAddr != 0x8001 || m.PlayAddr != 0x8002 {
		t.Errorf("INFO addr failed")
	}
	if m.Region != RegionPAL || m.ExtraChips != 2 || m.TotalSongs != 3 || m.StartSong != 5 {
		t.Errorf("INFO misc failed: region=%d chips=%d total=%d start=%d", m.Region, m.ExtraChips, m.TotalSongs, m.StartSong)
	}
}
