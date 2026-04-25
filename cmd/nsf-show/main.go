package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-pa/nsf"
)

var recursive = flag.Bool("r", false, "recursive search")

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [-r] <file or directory>...\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	for _, arg := range args {
		err := processPath(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error processing %s: %v\n", arg, err)
		}
	}
}

func processPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		if !*recursive {
			return fmt.Errorf("%s is a directory (use -r to recurse)", path)
		}
		return filepath.Walk(path, func(p string, i os.FileInfo, e error) error {
			if e != nil {
				return e
			}
			if !i.IsDir() && isNSFFile(p) {
				if err := showMetadata(p); err != nil {
					fmt.Fprintf(os.Stderr, "error processing %s: %v\n", p, err)
				}
			}
			return nil
		})
	}

	return showMetadata(path)
}

func isNSFFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".nsf" || ext == ".nsfe"
}

func showMetadata(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	m, err := nsf.Parse(f)
	if err != nil {
		return err
	}

	fmt.Printf("File: %s\n", path)
	fmt.Printf("  Format:       %s\n", m.Format)
	fmt.Printf("  Version:      %d\n", m.Version)
	fmt.Printf("  Total Songs:  %d\n", m.TotalSongs)
	fmt.Printf("  Start Song:   %d\n", m.StartSong)
	fmt.Printf("  Load Addr:    0x%04X\n", m.LoadAddr)
	fmt.Printf("  Init Addr:    0x%04X\n", m.InitAddr)
	fmt.Printf("  Play Addr:    0x%04X\n", m.PlayAddr)
	fmt.Printf("  Title:        %s\n", m.Title)
	fmt.Printf("  Artist:       %s\n", m.Artist)
	fmt.Printf("  Copyright:    %s\n", m.Copyright)
	if m.Ripper != "" {
		fmt.Printf("  Ripper:       %s\n", m.Ripper)
	}
	if m.Message != "" {
		fmt.Printf("  Message:      %s\n", m.Message)
	}
	fmt.Printf("  NTSC Speed:   %d\n", m.NTSCSpeed)
	fmt.Printf("  PAL Speed:    %d\n", m.PALSpeed)
	if m.DendySpeed > 0 {
		fmt.Printf("  Dendy Speed:  %d\n", m.DendySpeed)
	}
	fmt.Printf("  Region:       %s\n", m.FullRegionString())
	fmt.Printf("  Extra Chips:  %s\n", m.ExtraChips)
	fmt.Printf("  Bank Values:  %02X %02X %02X %02X %02X %02X %02X %02X\n",
		m.BankValues[0], m.BankValues[1], m.BankValues[2], m.BankValues[3],
		m.BankValues[4], m.BankValues[5], m.BankValues[6], m.BankValues[7])

	if m.HasNSF2Meta {
		fmt.Printf("  NSF2 Flags:   %s (0x%02X)\n", m.NSF2Flags, byte(m.NSF2Flags))
		fmt.Printf("  NSF2 DataLen: %d\n", m.NSF2DataLen)
	}

	tracks := m.PlaylistTracks()
	if len(tracks) > 0 {
		fmt.Printf("  Tracks:\n")
		for _, t := range tracks {
			line := fmt.Sprintf("    %3d:", t.Number)
			if t.Label != "" {
				line += " " + t.Label
			}
			if t.Author != "" {
				line += " (by " + t.Author + ")"
			}
			if dur := t.Duration(); dur != "" {
				line += " [" + dur + "]"
			}
			fmt.Println(line)
		}
	}

	tracks = m.SFXPlaylistTracks()
	if len(tracks) > 0 {
		fmt.Printf("  SFX Tracks:\n")
		for _, t := range tracks {
			line := fmt.Sprintf("    %3d:", t.Number)
			if t.Label != "" {
				line += " " + t.Label
			}
			if dur := t.Duration(); dur != "" {
				line += " (" + dur + ")"
			}
			fmt.Println(line)
		}
	}

	fmt.Println()

	return nil
}
