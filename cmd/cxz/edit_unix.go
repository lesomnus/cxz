//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"

	"github.com/lesomnus/cxz/internal/filemap"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli"
)

func localEditHandler(fn func(context.Context, *xli.Command) error) xli.Handler {
	return onRun(fn)
}

func editCommand() *xli.Command {
	return &xli.Command{Name: "edit", Brief: "Edit settings.jsonc in $VISUAL/$EDITOR; validate and sync changed settings", Commands: xli.Commands{editShareCommand(), {Name: "docker-compose", Brief: "Edit the project's shared Compose override file", Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		root := stateFrom(ctx)
		path, changed, err := editDockerCompose(root, func(path string) error { return openSettingsEditor(ctx, c, path) })
		if err != nil {
			return err
		}
		if changed {
			fmt.Fprintln(c.Writer, "Saved", path)
		} else {
			fmt.Fprintln(c.Writer, "No changes:", path)
		}
		cfg, err := settings.Load(root)
		if err != nil {
			return err
		}
		out, err := publishDevcontainer(ctx, root, cfg)
		if err != nil {
			return err
		}
		return writeOutput(c, out)
	})}}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		root := stateFrom(ctx)
		previous, _ := settings.Load(root)
		changed, bundle, err := editSettings(root, func(path string) error { return openSettingsEditor(ctx, c, path) })
		if err != nil {
			return err
		}
		if !changed {
			fmt.Fprintln(c.Writer, "No changes.")
			return nil
		}
		fmt.Fprintln(c.Writer, "Saved", settings.Path(root))
		current, err := settings.Load(root)
		if err != nil {
			return err
		}
		if current.Docker != previous.Docker {
			out, err := publishDocker(ctx, root, "save")
			if err != nil {
				return err
			}
			if err := writeOutput(c, out); err != nil {
				return err
			}
		}
		if !reflect.DeepEqual(current.Devcontainer, previous.Devcontainer) {
			if selectedComposeFileMissing(root, current) {
				fmt.Fprintln(c.Writer, "Run cxz edit docker-compose to create the selected Compose file.")
			} else {
				out, err := publishDevcontainer(ctx, root, current)
				if err != nil {
					return err
				}
				if err := writeOutput(c, out); err != nil {
					return err
				}
			}
		}
		if bundle != nil {
			out, err := publishFileMappings(ctx, root, *bundle)
			if err != nil {
				return err
			}
			return writeOutput(c, out)
		}
		return nil
	})}
}

func openSettingsEditor(ctx context.Context, c *xli.Command, path string) error {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		for _, name := range []string{"nano", "vim", "vi"} {
			if _, err := exec.LookPath(name); err == nil {
				editor = name
				break
			}
		}
	}
	if editor == "" {
		return fmt.Errorf("no editor found; set VISUAL or EDITOR")
	}
	// The editor variable is a user-supplied command. Pass the filename separately
	// so spaces, dollar signs, and shell syntax in state paths remain literal.
	cmd := exec.CommandContext(ctx, "sh", "-c", editor+` "$@"`, "cxz-editor", path)
	cmd.Stdin = c.ReadCloser
	cmd.Stdout = c.Writer
	cmd.Stderr = c.ErrWriter
	return cmd.Run()
}

func prepareSettingsEdit(root string, original []byte, cfg settings.Config) (*filemap.Bundle, error) {
	previous, oldErr := settings.Parse(original)
	if !reflect.DeepEqual(cfg.Devcontainer, previous.Devcontainer) && !selectedComposeFileMissing(root, cfg) {
		if _, err := cfg.Devcontainer.Snapshot(root); err != nil {
			return nil, err
		}
	}
	if cfg.Docker != previous.Docker {
		if _, err := cfg.Docker.Snapshot(root); err != nil {
			return nil, err
		}
	}
	var bundle *filemap.Bundle
	if (oldErr != nil && len(cfg.Files) > 0) || !slices.Equal(previous.Files, cfg.Files) {
		b, err := filemap.Snapshot(root, cfg.Files)
		if err != nil {
			return nil, err
		}
		bundle = &b
	}
	return bundle, nil
}

// Allow selecting a new path before the Compose editor creates it. Missing
// includes inside an existing file still fail validation.
func selectedComposeFileMissing(root string, cfg settings.Config) bool {
	if cfg.Devcontainer.Compose == "" || len(cfg.Devcontainer.LegacyInline) > 0 {
		return false
	}
	path, err := cfg.Devcontainer.Path(root)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return os.IsNotExist(err)
}

func finishSharedEdit(ctx context.Context, c *xli.Command, root, path string) error {
	cfg, err := settings.Load(root)
	if err != nil {
		return fmt.Errorf("shared file saved; cannot load mappings: %w", err)
	}
	bundle, err := sharedEditMappings(root, path, cfg.Files)
	if err != nil {
		return fmt.Errorf("shared file saved; sync mappings with cxz config files sync after correcting sources: %w", err)
	}
	if bundle == nil {
		return nil
	}
	out, err := publishFileMappings(ctx, root, *bundle)
	if err != nil {
		return err
	}
	return writeOutput(c, out)
}
