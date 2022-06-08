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

type linematcher uint

func (m linematcher) matchFrom(r *counterReader) bool {
	return uint(m) == r.lineno
}

type rxmatcher struct{ *regexp.Regexp }

func (m rxmatcher) matchFrom(r *counterReader) bool {
	return m.Match(r.lastb)
}

type counterReader struct {
	*bufio.Reader
	lastb               []byte
	lineno              uint
	lineoffset, lineend int64
}

func (r *counterReader) readBytes() (n int, err error) {
	r.lastb, err = r.Reader.ReadBytes('\n')
	n = len(r.lastb)
	if err == nil || n > 0 {
		r.lineno++
	}
	r.lineoffset = r.lineend
	r.lineend += int64(n)
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
		n, err := reader.readBytes()

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
		err = os.Remove(c.dst)
		if err != nil {
			return fmt.Errorf("split place not found, %w", err)
		}
		return fmt.Errorf("split place not found, removed file %s", c.dst)
	}

	if reader.lineno == 1 {
		ofile.Close()
		os.Remove(c.dst)
		if err != nil {
			return fmt.Errorf("not splitting at line 1, %w", err)
		}
		return fmt.Errorf("not splitting at line 1, removed file %s", c.dst)
	}

	err = ofile.Close()
	if err != nil {
		return fmt.Errorf("close split file: %w", err)
	}

	_, err = ifile.Seek(reader.lineoffset, 0)
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
	err = os.Rename(tfile.Name(), ifile.Name())
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

func splitDry(c *config) (err error) {
	var ifile *os.File

	ifile, err = os.Open(c.src)
	if err != nil {
		return err
	}
	defer ifile.Close()

	var (
		reader = counterReader{Reader: bufio.NewReader(ifile)}
		found  bool
		ocount int64
	)

	for {
		n, err := reader.readBytes()

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

		ocount += int64(n)
	}

	if !found {
		return fmt.Errorf("split place not found")
	}

	if reader.lineno == 1 {
		return fmt.Errorf("not splitting at line 1")
	}

	fmt.Printf("would split file %s at line %d, offset %d\n",
		c.src, reader.lineno, reader.lineoffset)
	fmt.Printf("line peek: `%s`\n", string(peek(reader.lastb, 62)))

	fmt.Printf("would write %d bytes to file %s\n", ocount, c.dst)

	_, err = ifile.Seek(reader.lineoffset, 0)
	if err != nil {
		return fmt.Errorf("seek input file: %w", err)
	}

	var tcount int64

	tcount, err = io.Copy(io.Discard, ifile)
	if err != nil {
		return fmt.Errorf("copy split to simulated temp: %w", err)
	}
	fmt.Printf("would rewrite file %s to %d bytes\n", c.src, tcount)

	return nil
}

func peek(b []byte, max int) []byte {
	b = chomp(b)
	if max < len(b) {
		return b[:max]
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
