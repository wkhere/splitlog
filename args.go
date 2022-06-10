package main

import (
	"fmt"
	"io"
	"regexp"

	"github.com/spf13/pflag"
)

func parseArgs(args []string) (c config, _ error) {
	var (
		line    uint
		pattern string
		back    uint
		help    bool
	)

	flag := pflag.NewFlagSet("flags", pflag.ContinueOnError)
	flag.SortFlags = false

	flag.UintVarP(&line, "line", "i", 0,
		"split at i-th line")
	flag.StringVarP(&pattern, "pattern", "p", "",
		"split at given regexp pattern")
	flag.UintVarP(&back, "back-from-match", "b", 0,
		fmt.Sprintf(
			"number of lines to go back from the match, max=%d",
			maxLinesBack,
		))
	flag.BoolVarP(&c.overwrite, "force", "f", false,
		"force overwriting SPLIT file if exists")
	flag.BoolVarP(&c.dryrun, "dry-run", "n", false,
		"do not change files, show what would be changed")

	flag.BoolVarP(&help, "help", "h", false,
		"show this help and exit")

	flag.Usage = func() {
		p := func(a ...interface{}) { fmt.Fprintln(flag.Output(), a...) }
		p("Usage: splitlog [FLAGS] FILE [SPLIT]")
		p("Split FILE at given position,",
			"writing earlier lines to SPLIT file and removing\n",
			"them from FILE.")
		p("If SPLIT is omitted, `FILE.1` will be used.")
		flag.PrintDefaults()
		p("One (and only one) of -i, -p flags must be used.")
		p("Note that original FILE can be rewritten even without -f flag.")
	}

	err := flag.Parse(args)
	if err != nil {
		return c, err
	}
	if help {
		c.help = func(w io.Writer) {
			flag.SetOutput(w)
			flag.Usage()
		}
		return c, nil
	}

	switch {
	case flag.Changed("line") == flag.Changed("pattern"):
		return c, fmt.Errorf("need -i or -p")

	case flag.Changed("line") && line < 2:
		return c, fmt.Errorf("split does not make sense at line < 2")

	case flag.Changed("line"):
		c.matcher = linematcher(line)

	case flag.Changed("pattern"):
		m := rxmatcher{}
		m.Regexp, err = regexp.Compile(pattern)
		if err != nil {
			return c, err
		}
		c.matcher = m
	}

	if flag.Changed("back-from-match") {
		if back > maxLinesBack {
			return c, fmt.Errorf("max value for -b is %d", maxLinesBack)
		}
		c.nLinesBack = int(back)
	}

	rest := flag.Args()
	switch n := len(rest); {
	case n == 1:
		c.src = rest[0]
		c.dst = c.src + ".1"
	case n == 2:
		c.src, c.dst = rest[0], rest[1]
	default:
		return c, fmt.Errorf("need FILE and optionally SPLIT file")
	}

	return c, nil
}
