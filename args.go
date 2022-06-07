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
		help    bool
	)

	flag := pflag.NewFlagSet("flags", pflag.ContinueOnError)
	flag.SortFlags = false

	flag.UintVarP(&line, "line", "i", 0,
		"split at i-th line")
	flag.StringVarP(&pattern, "pattern", "p", "",
		"split at given pattern")
	flag.BoolVarP(&c.overwrite, "force", "f", false,
		"force overwriting FILE2 if exists")
	flag.BoolVarP(&c.dryrun, "dry-run", "n", false,
		"do not change files, write what would be changed")

	flag.BoolVarP(&help, "help", "h", false,
		"show this help and exit")

	flag.Usage = func() {
		p := func(a ...interface{}) { fmt.Fprintln(flag.Output(), a...) }
		p("Usage: splitlog [FLAGS] FILE [FILE2]")
		p("Split FILE at given position,",
			"writing earlier lines to FILE2 and removing them\nfrom FILE.")
		p("If FILE2 is omitted, FILE.1 wii be used.")
		flag.PrintDefaults()
		p("One (and only one) of -i, -p flags must be used.")
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

	rest := flag.Args()
	switch n := len(rest); {
	case n == 1:
		c.from = rest[0]
		c.to = c.from + ".1"
	case n == 2:
		c.from, c.to = rest[0], rest[1]
	default:
		return c, fmt.Errorf("need FILE and optionally FILE2")
	}

	return c, nil
}
