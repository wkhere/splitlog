package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
)

type matcher interface {
	matchFrom(*counterReader) bool
}

type linematcher int

func (m linematcher) matchFrom(r *counterReader) bool {
	return int(m) == r.lineno
}

type rxmatcher struct{ *regexp.Regexp }

func (m rxmatcher) matchFrom(r *counterReader) bool {
	return m.Match(r.lastb)
}

const (
	maxLinesBack = 5
	previewLines = 2 // before and after match; must be < maxLinesBack
	maxPeekSize  = 76
)

type counterReader struct {
	*bufio.Reader
	lastb       []byte
	lineno      int
	lineoffsets [maxLinesBack + 1]int64
	lineend     int64
}

type peeksReader struct {
	*counterReader
	linepeeks [maxLinesBack + 1][]byte
}

func (r *counterReader) readBytes() (n int, err error) {
	r.lastb, err = r.Reader.ReadBytes('\n')
	n = len(r.lastb)
	if err == nil || n > 0 {
		r.lineno++
	}
	copy(r.lineoffsets[1:], r.lineoffsets[:])
	r.lineoffsets[0] = r.lineend
	r.lineend += int64(n)

	return n, err
}

func (r *peeksReader) readBytes() (n int, err error) {
	n, err = r.counterReader.readBytes()

	copy(r.linepeeks[1:], r.linepeeks[:])
	r.linepeeks[0] = peek(r.lastb, maxPeekSize)

	return n, err
}

func split(c *config) (err error) {
	if c.dryrun {
		return splitDry(c)
	}
	return splitReal(c)
}

func splitReal(c *config) (err error) {
	var ifile, ofile *os.File

	ifile, err = os.Open(c.src)
	if err != nil {
		return err
	}
	defer ifile.Close()

	ofile, err = os.OpenFile(c.dst, wfileflag(c.overwrite), 0600)

	if err != nil {
		return err
	}
	defer ofile.Close() // Close after err; valid path checks Close retval

	var (
		reader = counterReader{Reader: bufio.NewReader(ifile)}
		found  bool
	)

	for {
		var n int
		n, err = reader.readBytes()

		if err != nil && err != io.EOF {
			return fmt.Errorf("read to find split place: %w", err)
		}
		if err == io.EOF && n == 0 {
			break
		}

		if c.matcher.matchFrom(&reader) {
			found = true
			break
		}

		_, err = ofile.Write(reader.lastb)
		if err != nil {
			return fmt.Errorf("write split: %w", err)
		}
	}

	if !found {
		ofile.Close()
		return removeSplit(c.dst, "split place not found")
	}

	var (
		splitline   = reader.lineno - c.nLinesBack
		splitoffset = reader.lineoffsets[c.nLinesBack]
	)

	if splitline < 1 {
		ofile.Close()
		return removeSplit(c.dst, "not splitting at line < 1")
	}
	if splitline == 1 {
		ofile.Close()
		return removeSplit(c.dst, "not splitting at line 1")
	}
	if c.nLinesBack > 0 {
		err = ofile.Truncate(splitoffset)
		if err != nil {
			return fmt.Errorf("truncate split file: %w", err)
		}
	}

	err = ofile.Close()
	if err != nil {
		return fmt.Errorf("close split file: %w", err)
	}

	_, err = ifile.Seek(splitoffset, 0)
	if err != nil {
		return fmt.Errorf("seek input file: %w", err)
	}

	var tfile *os.File

	tfile, err = os.CreateTemp(".", c.src+".split")
	if err != nil {
		return fmt.Errorf("tempfile: %w", err)
	}
	defer tfile.Close() // Close after err; valid path checks Close retval

	_, err = io.Copy(tfile, ifile)
	if err != nil {
		return fmt.Errorf("copy inputfile tail to tempfile: %w", err)
	}
	ifile.Close()
	err = tfile.Close()
	if err != nil {
		return fmt.Errorf("close tempfile: %w", err)
	}
	err = os.Rename(tfile.Name(), c.src)
	if err != nil {
		return fmt.Errorf("rename tempfile to orig: %w", err)
	}

	return nil
}

func wfileflag(overwrite bool) (flag int) {
	flag = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	if !overwrite {
		flag |= os.O_EXCL
	}
	return
}

func removeSplit(file, reason string) error {
	err := os.Remove(file)
	if err != nil {
		return fmt.Errorf("%s, %w", reason, err)
	}
	return fmt.Errorf("%s, removed file %s", reason, file)
}

func splitDry(c *config) (err error) {
	var ifile *os.File

	ifile, err = os.Open(c.src)
	if err != nil {
		return err
	}
	defer ifile.Close()

	var (
		reader = peeksReader{
			counterReader: &counterReader{
				Reader: bufio.NewReader(ifile),
			},
		}
		found  bool
		ocount int64
	)

	for {
		var n int
		n, err = reader.readBytes()

		if err != nil && err != io.EOF {
			return fmt.Errorf("read to find split place: %w", err)
		}
		if err == io.EOF && n == 0 {
			break
		}

		if c.matcher.matchFrom(reader.counterReader) {
			found = true
			break
		}

		ocount += int64(n)
	}

	if !found {
		return fmt.Errorf("split place not found")
	}

	var (
		matchline   = reader.lineno
		splitline   = reader.lineno - c.nLinesBack
		splitoffset = reader.lineoffsets[c.nLinesBack]
	)
	if splitline < 1 {
		return fmt.Errorf("not splitting at line < 1")
	}
	if splitline == 1 {
		return fmt.Errorf("not splitting at line 1")
	}
	if c.nLinesBack > 0 {
		ocount = splitoffset
	}

	fmt.Printf("* would split file %s at line %d, offset %d\n",
		c.src, splitline, splitoffset)
	if c.nLinesBack == 0 {
		fmt.Println("* preview, `>` shows the split")
	} else {
		fmt.Println("* preview, `>` shows the split, `=` shows the match")
	}

	{
		// Preview algo:
		// for pre-match lines, show either #nLinesBack or #previewLines
		// number of lines, which is bigger. Make correction for a case
		// when such number of lines was not read at all before match.
		// Can't happen with nLinesBack, as it would be exited above,
		// but can happen with #previewLines.
		// Mark split line on the way.
		// Show match line, with a mark.
		// Show #previewLines next lines, ofc if they exist in the file
		// (which we don't know yet atm).
		// For the copy simulation, file will be rewinded to the split line
		// anyway.
		var ipreview int // 1st index in the peeks table, going down

		ipreview = max(c.nLinesBack, previewLines)
		ipreview = min(ipreview, matchline-1)

		for i := ipreview; i >= 0; i-- {
			switch i {
			case c.nLinesBack:
				fmt.Print("> ")
			case 0:
				fmt.Print("= ")
			default:
				fmt.Print("  ")
			}
			fmt.Printf("%s\n", reader.linepeeks[i])
		}
		// post-match lines, reuse the reader which is to be discarded anyway
		for i := 0; i < previewLines; i++ {
			_, err = reader.readBytes()
			if err != nil && err != io.EOF {
				return fmt.Errorf("read input file past the match: %w", err)
			}
			fmt.Printf("  %s\n", reader.linepeeks[0])
		}
	}

	fmt.Printf("* would write %d bytes to file %s\n", ocount, c.dst)

	_, err = ifile.Seek(splitoffset, 0)
	if err != nil {
		return fmt.Errorf("seek input file: %w", err)
	}

	var tcount int64

	tcount, err = io.Copy(io.Discard, ifile)
	if err != nil {
		return fmt.Errorf("copy split to simulated temp: %w", err)
	}
	fmt.Printf("* would rewrite file %s to %d bytes\n", c.src, tcount)

	return nil
}

func peek(b []byte, max int) []byte {
	b = chomp(b)
	if max < len(b) {
		return append(b[:max], []byte("..")...)
	}
	return b
}

func chomp(b []byte) []byte {
	for len(b) > 0 {
		l := len(b) - 1
		if b[l] == '\n' || b[l] == '\r' {
			b = b[:l]
		} else {
			break
		}
	}
	return b
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}
