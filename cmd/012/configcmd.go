package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/keyring"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// runConfig is `012 config ...`: see docs/reference/config.md.
func runConfig(args []string, e env) error {
	cfg, err := loadConfig(e, nil)
	if err != nil {
		return err
	}
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	if len(args) > 1 {
		return usage()
	}
	switch sub {
	case "":
		fmt.Fprint(e.stdout, cfg.Show())
		fmt.Fprintf(e.stdout, "\n# JEV API key: %s\n", keyStatus(cfg, e))
	case "path":
		fmt.Fprintln(e.stdout, cfg.Path)
	case "default":
		fmt.Fprint(e.stdout, config.DefaultFile())
	case "edit":
		return editConfig(cfg.Path, e)
	case "themes":
		listThemes(cfg, e)
	case "set-key":
		return setKey(cfg, e)
	case "delete-key":
		return deleteKey(cfg, e)
	default:
		return usage()
	}
	return nil
}

// keyStatus says where the API key would come from, without showing it
// or running jev-api-key-command.
func keyStatus(c *config.Config, e env) string {
	if strings.TrimSpace(e.getenv(jev.KeyEnv)) != "" {
		return "from $" + jev.KeyEnv
	}
	if store := keyStore(c, e); store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := store.Get(ctx)
		switch {
		case err == nil:
			return "stored in the " + store.Name()
		case !errors.Is(err, keyring.ErrNotFound):
			return "the " + store.Name() + " couldn't be read: " + err.Error()
		}
	}
	if c.String("jev-api-key-command") != "" {
		return "from jev-api-key-command (not run here)"
	}
	return "not set; run 012 config set-key"
}

func editConfig(path string, e env) error {
	if err := config.EnsureFile(path); err != nil {
		return err
	}
	cmd, err := config.Editor(path, e.getenv)
	if err != nil {
		return err
	}
	if e.editorIO {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	}
	return cmd.Run()
}

func listThemes(c *config.Config, e env) {
	current := c.Theme()
	for _, t := range theme.List(config.ThemesDir()) {
		kind := "light"
		if t.Dark {
			kind = "dark"
		}
		switch {
		case t.Name == theme.Terminal:
			kind = "the terminal's own colors"
		case t.Name == theme.HighContrast:
			kind = "high contrast, dark or light by the terminal"
		case t.User:
			kind += ", " + config.ThemesDir()
		}
		mark := " "
		if strings.EqualFold(t.Name, current.Light) || strings.EqualFold(t.Name, current.Dark) {
			mark = "*"
		}
		fmt.Fprintf(e.stdout, "%s %-36s %s\n", mark, t.Name, kind)
	}
}

// setKey stores the API key read from the terminal, without echo, or
// from stdin when it's a pipe. The key is never an argument.
func setKey(c *config.Config, e env) error {
	store := keyStore(c, e)
	if store == nil {
		return errors.New("jev-credential-store is off in the config; use jev-api-key-command or TYPESAFE_API_KEY instead")
	}
	var key string
	var err error
	if e.isTTY {
		key, err = e.readKey("TypeSafe API key (not shown): ")
	} else {
		key, err = firstLine(e.stdin)
	}
	if err != nil {
		return err
	}
	if key = strings.TrimSpace(key); key == "" {
		return errors.New("no key given; nothing changed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := store.Set(ctx, key); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "Saved the JEV API key in the %s.\n", store.Name())
	if strings.TrimSpace(e.getenv(jev.KeyEnv)) != "" {
		fmt.Fprintf(e.stdout, "$%s is set too, and wins while it is.\n", jev.KeyEnv)
	}
	return nil
}

func deleteKey(c *config.Config, e env) error {
	store := keyStore(c, e)
	if store == nil {
		return errors.New("jev-credential-store is off in the config")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch err := store.Delete(ctx); {
	case errors.Is(err, keyring.ErrNotFound):
		fmt.Fprintf(e.stdout, "No JEV API key in the %s.\n", store.Name())
	case err != nil:
		return err
	default:
		fmt.Fprintf(e.stdout, "Deleted the JEV API key from the %s.\n", store.Name())
	}
	return nil
}

func firstLine(r interface{ Read([]byte) (int, error) }) (string, error) {
	sc := bufio.NewScanner(r)
	if sc.Scan() {
		return sc.Text(), nil
	}
	return "", sc.Err()
}

func isTerminal(f *os.File) bool { return term.IsTerminal(f.Fd()) }

// readPassword prompts on stderr and reads a line from the terminal with
// echo off.
func readPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(os.Stdin.Fd())
	fmt.Fprintln(os.Stderr)
	return string(b), err
}
