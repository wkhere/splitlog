package main

import (
	"fmt"
	"io"
	"os"
)

type config struct {
	matcher    matcher
	src, dst   string
	overwrite  bool
	nLinesBack int

	dryrun      bool
	widePreview bool

	help func(io.Writer)
}

func main() {
	c, err := parseArgs(os.Args[1:])
	if err != nil {
		die(2, err)
	}
	if c.help != nil {
		c.help(os.Stdout)
		os.Exit(0)
	}

	st1, err := os.Stat(c.src)
	switch {
	case err != nil:
		die(1, fmt.Errorf("file %s not found", c.src))
	case st1.IsDir():
		die(1, fmt.Errorf("%s is a dir", c.src))
	}
	st2, err := os.Stat(c.dst)
	switch {
	case os.SameFile(st1, st2):
		die(1, fmt.Errorf("%s and %s are the same file", c.src, c.dst))
	case !c.overwrite && !c.dryrun && !os.IsNotExist(err):
		die(1, fmt.Errorf("%s already exists", c.dst))
	}

	err = split(&c)
	if err != nil {
		die(1, err)
	}
}

func die(exitcode int, msgs ...interface{}) {
	log(msgs...)
	os.Exit(exitcode)
}

func log(msgs ...interface{}) {
	if len(msgs) > 0 {
		fmt.Fprint(os.Stderr, "splitlog: ")
		fmt.Fprintln(os.Stderr, msgs...)
	}
}
