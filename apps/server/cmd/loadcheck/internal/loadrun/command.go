package loadrun

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"time"
)

func Command(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "run" {
		return errors.New("usage: loadcheck run [options]")
	}
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var c Config
	var phases, queriesFile, sessions, emails string
	flags.StringVar(&c.Pages, "pages", "", "pages origin")
	flags.StringVar(&c.API, "api", "", "API origin")
	flags.StringVar(&c.PublicURL, "public-url", "", "public origin")
	flags.StringVar(&queriesFile, "queries", "", "queries file")
	flags.StringVar(&c.Google, "google", "", "fake Google origin")
	flags.StringVar(&emails, "emails", "", "account emails")
	flags.StringVar(&sessions, "sessions", "", "session files")
	flags.StringVar(&c.Out, "out", "", "output directory")
	flags.StringVar(&c.Activate, "activate", "", "activation command")
	flags.StringVar(&phases, "phases", "A,B,C,D", "phases")
	flags.DurationVar(&c.DurationA, "duration-a", 10*time.Minute, "phase A duration")
	flags.DurationVar(&c.DurationC, "duration-c", 5*time.Minute, "phase C duration")
	flags.DurationVar(&c.DurationD, "duration-d", 5*time.Minute, "phase D duration")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return errors.New("invalid run options")
	}
	if queriesFile == "" {
		return errors.New("queries file required")
	}
	file, err := os.Open(queriesFile)
	if err != nil {
		return errors.New("open queries file failed")
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		c.Queries = append(c.Queries, strings.TrimSpace(scanner.Text()))
	}
	if scanner.Err() != nil {
		return errors.New("read queries file failed")
	}
	c.Phases = strings.Split(phases, ",")
	if emails != "" {
		c.Emails = strings.Split(emails, ",")
	}
	if sessions != "" {
		c.Sessions = strings.Split(sessions, ",")
	}
	return Run(ctx, c)
}
