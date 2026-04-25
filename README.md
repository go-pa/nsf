# nsf

[![Go Reference](https://pkg.go.dev/badge/github.com/go-pa/nsf.svg)](https://pkg.go.dev/github.com/go-pa/nsf)

`nsf` is a Go library for working with Nintendo Sound Format (NSF) files.
It supports parsing metadata from NSF, NSFE (Extended NSF), and NSF2 files.

## Features

- **Format Support**: NSF, NSFE, and NSF2 formats.

## Example

The `cmd/nsf-show` directory contains a simple utility that demonstrates how to
use the library to display information about NSF/NSFE files.

```bash
go run ./cmd/nsf-show testdata/nsf/Akai\ Yousai\ -\ Final\ Commando\ \(FDS\).nsfe

File: testdata/nsf/Akai Yousai - Final Commando (FDS).nsfe
  Format:       NSFE
  Version:      0
  Total Songs:  9
  Start Song:   1
  Load Addr:    0xC000
  Init Addr:    0xC030
  Play Addr:    0xC48A
  Title:        Akai Yousai: Final Commando
  Artist:       Shinya Sakamoto, Atsushi Fujio
  Copyright:    ©1988 Konami
  Ripper:       (unknown)
  NTSC Speed:   0
  PAL Speed:    0
  Region:       NTSC
  Extra Chips:  FDS
  Bank Values:  00 00 00 00 00 00 00 00
  Tracks:
      1: Introduction [0:19 + 0:01 fade]
      2: Stages 1 & 4 [1:22 + 0:10 fade]
      5: Boss Theme 1 [0:38 + 0:10 fade]
      7: Stage Clear [0:10 + 0:01 fade]
      3: Stages 2 & 5 [2:46 + 0:10 fade]
      6: Boss Theme 2 [0:44 + 0:10 fade]
      4: Stages 3 & 6 [1:07 + 0:10 fade]
      8: Game Over [0:07 + 0:01 fade]
      9: Staff Roll [1:34 + 0:10 fade]
```

## License

MIT
