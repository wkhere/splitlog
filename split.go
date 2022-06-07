package main

import (
	"bufio"
	"errors"
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
	var ifile, ofile, tfile *os.File

	ifile, err = os.Open(c.from)
	if err != nil {
		return err
	}
	defer ifile.Close()

	ofile, err = os.OpenFile(c.to, wfileflag(c.overwrite), 0600)

	if err != nil {
		return err
	}
	defer func() {
		cerr := ofile.Close()
		if cerr != nil && !errors.Is(cerr, os.ErrClosed) && err == nil {
			err = cerr
		}
	}()

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
		err = os.Remove(c.to)
		if err != nil {
			return fmt.Errorf("split place not found, %w", err)
		}
		return fmt.Errorf("split place not found, removed file %s", c.to)
	}

	if reader.lineno == 1 {
		ofile.Close()
		os.Remove(c.to)
		if err != nil {
			return fmt.Errorf("not splitting at line 1, %w", err)
		}
		return fmt.Errorf("not splitting at line 1, removed file %s", c.to)
	}

	_, err = ifile.Seek(reader.lineoffset, 0)
	if err != nil {
		return fmt.Errorf("seek input file: %w", err)
	}

	tfile, err = os.CreateTemp(".", c.from+".split")
	if err != nil {
		return fmt.Errorf("tempfile: %w", err)
	}
	defer func() {
		cerr := tfile.Close()
		if cerr != nil && !errors.Is(cerr, os.ErrClosed) && err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(tfile, ifile)
	if err != nil {
		return fmt.Errorf("copy split to tempfile: %w", err)
	}
	ifile.Close()
	err = tfile.Close()
	if err != nil {
		return err
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

	ifile, err = os.Open(c.from)
	if err != nil {
		return err
	}
	defer ifile.Close()

	var (
		reader = counterReader{Reader: bufio.NewReader(ifile)}
		found  bool
		ocount int64
		tcount int64
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
		c.from, reader.lineno, reader.lineoffset)

	fmt.Printf("would write %d bytes to file %s\n", ocount, c.to)

	_, err = ifile.Seek(reader.lineoffset, 0)
	if err != nil {
		return fmt.Errorf("seek input file: %w", err)
	}

	tcount, err = io.Copy(io.Discard, ifile)
	if err != nil {
		return fmt.Errorf("copy split to simulated temp: %w", err)
	}
	fmt.Printf("would rewrite file %s to %d bytes\n", c.from, tcount)

	return nil
}
