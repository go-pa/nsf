package nsf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Track represents metadata for a single track (song) in an NSF file.
type Track struct {
	Number int    // Track number
	Label  string // Individual track label (NSFe/NSF2 only)
	Time   int    // Individual track duration in milliseconds (NSFe/NSF2 only)
	Fade   int    // Individual track fade-out duration in milliseconds (NSFe/NSF2 only)
	Author string // Individual track author (NSFe/NSF2 only)
}

func (t Track) IsZero() bool {
	return t == Track{}
}

// Duration returns a human-readable duration string for the track.
func (t Track) Duration() string {
	if t.Time <= 0 && t.Fade <= 0 {
		return ""
	}
	dur := ""
	if t.Time > 0 {
		dur = FormatDuration(t.Time)
	}
	if t.Fade > 0 {
		if dur != "" {
			dur += " + "
		}
		dur += FormatDuration(t.Fade) + " fade"
	}
	return dur
}

// Metadata represents the metadata found in an NSF, NSFE, or NSF2 file.
type Metadata struct {
	Format     Format        // NSF or NSFE
	Version    byte          // 1 or 2 (for NSF)
	TotalSongs int           // Total number of songs in the file
	StartSong  int           // Starting song number (1-indexed)
	LoadAddr   uint16        // Address to load ROM data
	InitAddr   uint16        // Address to call to initialize a song
	PlayAddr   uint16        // Address to call to play a frame of music
	Title      string        // Title of the song/game
	Artist     string        // Artist name
	Copyright  string        // Copyright information
	Ripper     string        // Ripper name (NSFe/NSF2 only)
	Message    string        // Message/description (NSFe/NSF2 'text' chunk)
	NTSCSpeed  uint16        // NTSC play speed (microseconds per frame)
	PALSpeed   uint16        // PAL play speed (microseconds per frame)
	DendySpeed uint16        // Dendy play speed (microseconds per frame)
	Region     Region        // Region bitmask
	PrefRegion Region        // Preferred region (single bit)
	ExtraChips ExpansionChip // Expansion chips used
	BankValues [8]byte       // Initial bank switching values
	Tracks     []Track       // Individual track information (NSFe/NSF2 only)
	ROMData    []byte        // Raw ROM data (NSF/NSFE/NSF2)

	// NSF2 specific fields
	NSF2Flags    NSF2Flag // NSF2 feature flags
	NSF2DataLen  int      // Length of the ROM data before metadata chunks
	HasNSF2Meta  bool     // True if the file contains appended NSF2 metadata
	NSF2MetaData []byte   // Raw metadata chunks (if not fully parsed)
	VRC7Data     []byte   // VRC7 patch data
	MixerData    []byte   // Mixer levels (NSFe 'mixe' chunk)

	Playlist    []int // Playlist track indices (NSFe only)
	SFXPlaylist []int // SFX playlist track indices (NSFe only)
}

// FullRegionString returns a string representation of the region(s) supported
// by the file, including the preferred region if specified.
func (m *Metadata) FullRegionString() string {
	s := m.Region.String()
	if m.PrefRegion != 0 {
		s += " (Preferred: " + m.PrefRegion.String() + ")"
	}
	return s
}

func (m *Metadata) track(i int) *Track {
	if n := i + 1 - len(m.Tracks); n > 0 {
		start := len(m.Tracks)
		m.Tracks = append(m.Tracks, make([]Track, n)...)
		for j := start; j <= i; j++ {
			m.Tracks[j].Number = j + 1
		}
	}
	return &m.Tracks[i]
}

func (m *Metadata) getTrack(i int) Track {
	if i < len(m.Tracks) {
		return m.Tracks[i]
	}
	return Track{Number: i + 1}
}

// PlaylistTracks returns all tracks in the order they should be played.
// If a playlist is present, only tracks listed in the playlist are returned
// (in first-appearance order, duplicates skipped).
// If there is no per-track metadata and no playlist, it returns nil.
func (m *Metadata) PlaylistTracks() []Track {
	if len(m.Tracks) == 0 && len(m.Playlist) == 0 {
		return nil
	}

	if len(m.Playlist) > 0 {
		return m.tracksForPlaylist(m.Playlist)
	}

	return m.Tracks
}

// SFXPlaylistTracks returns all tracks in the SFX playlist.
// If an SFX playlist is present, only tracks listed in the playlist are returned
// (in first-appearance order, duplicates skipped).
// If there is no SFX playlist, it returns nil.
func (m *Metadata) SFXPlaylistTracks() []Track {
	return m.tracksForPlaylist(m.SFXPlaylist)
}

func (m *Metadata) tracksForPlaylist(playlist []int) []Track {
	if len(playlist) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(playlist))
	indices := make([]int, 0, len(playlist))
	for _, idx := range playlist {
		if !seen[idx] {
			seen[idx] = true
			indices = append(indices, idx)
		}
	}
	res := make([]Track, len(indices))
	for i, idx := range indices {
		// TODO: instead of filling in an empty track maybe this should be som kind of error
		res[i] = m.getTrack(idx)
	}
	return res
}

var (
	ErrInvalidMagic = errors.New("invalid NSF/NSFE magic")
	ErrTruncated    = errors.New("truncated file")
)

// Parse parses the NSF, NSFE, or NSF2 metadata from the given reader.
func Parse(r io.Reader) (*Metadata, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseBytes(data)
}

// ParseBytes parses the NSF, NSFE, or NSF2 metadata from the given byte slice.
func ParseBytes(data []byte) (*Metadata, error) {
	if len(data) < 4 {
		return nil, ErrTruncated
	}

	magic := data[:4]
	if bytes.Equal(magic, []byte("NESM")) {
		return parseNSF(data[4:])
	} else if bytes.Equal(magic, []byte("NSFE")) {
		return parseNSFE(data[4:])
	}

	return nil, ErrInvalidMagic
}

func parseNSF(data []byte) (*Metadata, error) {
	if len(data) < 124 {
		return nil, ErrTruncated
	}

	header := data[:124]
	if header[0] != 0x1a {
		return nil, ErrInvalidMagic
	}

	m := &Metadata{
		Format:      FormatNSF,
		Version:     header[1],
		TotalSongs:  int(header[2]),
		StartSong:   int(header[3]),
		LoadAddr:    binary.LittleEndian.Uint16(header[4:6]),
		InitAddr:    binary.LittleEndian.Uint16(header[6:8]),
		PlayAddr:    binary.LittleEndian.Uint16(header[8:10]),
		Title:       parseNSFString(header[10:42]),
		Artist:      parseNSFString(header[42:74]),
		Copyright:   parseNSFString(header[74:106]),
		NTSCSpeed:   binary.LittleEndian.Uint16(header[106:108]),
		PALSpeed:    binary.LittleEndian.Uint16(header[116:118]),
		Region:      parseNSFRegion(header[118]),
		ExtraChips:  ExpansionChip(header[119]),
		NSF2Flags:   NSF2Flag(header[120]),
		NSF2DataLen: int(uint32(header[121]) | uint32(header[122])<<8 | uint32(header[123])<<16),
	}
	copy(m.BankValues[:], header[108:116])

	remaining := data[124:]

	// Read ROM data
	if m.Version >= 2 && m.NSF2DataLen > 0 {
		m.HasNSF2Meta = true
		if len(remaining) < m.NSF2DataLen {
			return nil, ErrTruncated
		}
		m.ROMData = remaining[:m.NSF2DataLen]
		m.parseChunks(remaining[m.NSF2DataLen:])
	} else {
		m.ROMData = remaining
	}
	return m, nil
}

func parseNSFRegion(b byte) Region {
	switch b & 0x03 {
	case 0:
		return RegionNTSC
	case 1:
		return RegionPAL
	case 2:
		return RegionNTSC | RegionPAL
	case 3:
		return RegionDendy
	default:
		return 0
	}
}

func parsePrefRegion(b byte) Region {
	switch b {
	case 0:
		return RegionNTSC
	case 1:
		return RegionPAL
	case 2:
		return RegionDendy
	default:
		return 0
	}
}

func (m *Metadata) parseCommonChunk(chunkType string, data []byte) {
	switch chunkType {
	case "auth":
		parts := bytes.Split(data, []byte{0})
		if len(parts) >= 1 && len(parts[0]) > 0 {
			m.Title = decodeString(parts[0])
		}
		if len(parts) >= 2 && len(parts[1]) > 0 {
			m.Artist = decodeString(parts[1])
		}
		if len(parts) >= 3 && len(parts[2]) > 0 {
			m.Copyright = decodeString(parts[2])
		}
		if len(parts) >= 4 && len(parts[3]) > 0 {
			m.Ripper = decodeString(parts[3])
		}
	case "plst":
		m.Playlist = make([]int, len(data))
		for i, b := range data {
			m.Playlist[i] = int(b)
		}
	case "psfx":
		m.SFXPlaylist = make([]int, len(data))
		for i, b := range data {
			m.SFXPlaylist[i] = int(b)
		}
	case "tlbl":
		parts := bytes.Split(data, []byte{0})
		for i, p := range parts {
			if len(p) > 0 {
				m.track(i).Label = decodeString(p)
			}
		}
	case "taut":
		parts := bytes.Split(data, []byte{0})
		for i, p := range parts {
			if len(p) > 0 {
				m.track(i).Author = decodeString(p)
			}
		}
	case "text":
		m.Message = decodeString(data)
	case "time":
		for i := range len(data) / 4 {
			m.track(i).Time = int(binary.LittleEndian.Uint32(data[i*4 : (i+1)*4]))
		}
	case "fade":
		for i := range len(data) / 4 {
			m.track(i).Fade = int(binary.LittleEndian.Uint32(data[i*4 : (i+1)*4]))
		}
	case "BANK":
		copy(m.BankValues[:], data)
	case "RATE":
		if len(data) >= 2 {
			m.NTSCSpeed = binary.LittleEndian.Uint16(data[0:2])
		}
		if len(data) >= 4 {
			m.PALSpeed = binary.LittleEndian.Uint16(data[2:4])
		}
		if len(data) >= 6 {
			m.DendySpeed = binary.LittleEndian.Uint16(data[4:6])
		}
	case "NSF2":
		if len(data) >= 1 {
			m.NSF2Flags = NSF2Flag(data[0])
		}
	case "regn":
		if len(data) >= 1 {
			m.Region = Region(data[0])
		}
		if len(data) >= 2 {
			m.PrefRegion = parsePrefRegion(data[1])
		}
	case "VRC7":
		m.VRC7Data = data
	case "mixe":
		m.MixerData = data
	case "DATA":
		m.ROMData = data
	}
}

func readChunk(data []byte) (chunkType string, chunkData []byte, remaining []byte, err error) {
	if len(data) == 0 {
		return "", nil, nil, io.EOF
	}
	if len(data) < 8 {
		return "", nil, nil, ErrTruncated
	}
	length := binary.LittleEndian.Uint32(data[0:4])
	chunkType = string(data[4:8])
	if uint32(len(data)) < 8+length {
		return "", nil, nil, ErrTruncated
	}
	chunkData = data[8 : 8+length]
	remaining = data[8+length:]
	return
}

func (m *Metadata) parseChunks(data []byte) error {
	for len(data) > 0 {
		chunkType, chunkData, remaining, err := readChunk(data)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		data = remaining
		if chunkType == "NEND" {
			return nil
		}
		m.parseCommonChunk(chunkType, chunkData)
	}
	return nil
}

func parseNSFE(data []byte) (*Metadata, error) {
	m := &Metadata{Format: FormatNSFE}
	return m, m.parseNSFEChunks(data)
}

func (m *Metadata) parseNSFEChunks(data []byte) error {
	for len(data) > 0 {
		chunkType, chunkData, remaining, err := readChunk(data)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		data = remaining
		switch chunkType {
		case "INFO":
			if len(chunkData) >= 2 {
				m.LoadAddr = binary.LittleEndian.Uint16(chunkData[0:2])
			}
			if len(chunkData) >= 4 {
				m.InitAddr = binary.LittleEndian.Uint16(chunkData[2:4])
			}
			if len(chunkData) >= 6 {
				m.PlayAddr = binary.LittleEndian.Uint16(chunkData[4:6])
			}
			if len(chunkData) >= 7 {
				m.Region = parseNSFRegion(chunkData[6])
			}
			if len(chunkData) >= 8 {
				m.ExtraChips = ExpansionChip(chunkData[7])
			}
			if len(chunkData) >= 9 {
				m.TotalSongs = int(chunkData[8])
			}
			if len(chunkData) >= 10 {
				m.StartSong = int(chunkData[9]) + 1
			}
		case "NEND":
			return nil
		default:
			m.parseCommonChunk(chunkType, chunkData)
		}
	}
	return nil
}

func decodeString(b []byte) string {
	s := string(b)
	if !utf8.Valid(b) {
		if d, err := charmap.Windows1252.NewDecoder().Bytes(b); err == nil {
			s = string(d)
		}
	}
	return strings.Map(func(r rune) rune {
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			return -1
		}
		return r
	}, s)
}

func parseNSFString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i != -1 {
		b = b[:i]
	}
	return decodeString(b)
}
