package main

import (
	"fmt"
	"io"
	"os"
)

type config struct {
	matcher   matcher
	from, to  string
	overwrite bool

	dryrun bool
	help   func(io.Writer)
}

func main() {
	c, err := parseArgs(os.Args[1:])
	if err != nil {
		die(2, err)
	}
	if c.help != nil {
		c.help(os.Stdout)
		die(0)
	}

	st1, err := os.Stat(c.from)
	switch {
	case err != nil:
		die(1, fmt.Errorf("%s not found", c.from))
	case st1.IsDir():
		die(1, fmt.Errorf("%s is a dir", c.from))
	}
	st2, err := os.Stat(c.to)
	switch {
	case os.SameFile(st1, st2):
		die(1, fmt.Errorf("%s and %s are the same file", c.from, c.to))
	case !c.overwrite && !c.dryrun && !os.IsNotExist(err):
		die(1, fmt.Errorf("%s already exists", c.to))
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
