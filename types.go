package nsf

import (
	"fmt"
	"strings"
)

type Format int

//go:generate go run github.com/dmarkham/enumer@v1.6.3 -type=Format -trimprefix=Format
const (
	FormatNSF  Format = iota // NSF
	FormatNSFE               // NSFE
)

type Region byte

const (
	RegionNTSC  Region = 1 << 0
	RegionPAL   Region = 1 << 1
	RegionDendy Region = 1 << 2
)

func (r Region) String() string {
	var regions []string
	if r&RegionNTSC != 0 {
		regions = append(regions, "NTSC")
	}
	if r&RegionPAL != 0 {
		regions = append(regions, "PAL")
	}
	if r&RegionDendy != 0 {
		regions = append(regions, "Dendy")
	}
	if len(regions) == 0 {
		return "Unknown"
	}
	return strings.Join(regions, ", ")
}

type ExpansionChip byte

const (
	ChipVRC6      ExpansionChip = 1 << 0
	ChipVRC7      ExpansionChip = 1 << 1
	ChipFDS       ExpansionChip = 1 << 2
	ChipMMC5      ExpansionChip = 1 << 3
	ChipNamco163  ExpansionChip = 1 << 4
	ChipSunsoft5B ExpansionChip = 1 << 5
	ChipVT02      ExpansionChip = 1 << 6 // VT02+
)

func (c ExpansionChip) String() string {
	if c == 0 {
		return "None"
	}
	var chips []string
	if c&ChipVRC6 != 0 {
		chips = append(chips, "VRC6")
	}
	if c&ChipVRC7 != 0 {
		chips = append(chips, "VRC7")
	}
	if c&ChipFDS != 0 {
		chips = append(chips, "FDS")
	}
	if c&ChipMMC5 != 0 {
		chips = append(chips, "MMC5")
	}
	if c&ChipNamco163 != 0 {
		chips = append(chips, "Namco 163")
	}
	if c&ChipSunsoft5B != 0 {
		chips = append(chips, "Sunsoft 5B")
	}
	if c&ChipVT02 != 0 {
		chips = append(chips, "VT02+")
	}
	return strings.Join(chips, ", ")
}

type NSF2Flag byte

const (
	NSF2FlagIRQ        NSF2Flag = 1 << 4
	NSF2FlagNonRetInit NSF2Flag = 1 << 5
	NSF2FlagNoPlay     NSF2Flag = 1 << 6
	NSF2FlagMandatory  NSF2Flag = 1 << 7
)

func (f NSF2Flag) String() string {
	var flags []string
	if f&NSF2FlagIRQ != 0 {
		flags = append(flags, "IRQ")
	}
	if f&NSF2FlagNonRetInit != 0 {
		flags = append(flags, "Non-Returning INIT")
	}
	if f&NSF2FlagNoPlay != 0 {
		flags = append(flags, "Suppressed PLAY")
	}
	if f&NSF2FlagMandatory != 0 {
		flags = append(flags, "Mandatory Metadata")
	}
	if len(flags) == 0 {
		return "None"
	}
	return strings.Join(flags, ", ")
}

func FormatDuration(ms int) string {
	s := ms / 1000
	m := s / 60
	s = s % 60
	return fmt.Sprintf("%d:%02d", m, s)
}
