//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/filemap"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/server"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func fileMappingsCommand() *xli.Command {
	group := commandGroup("files", "Copy host files/directories to agent containers")
	for _, op := range []string{"ls", "add", "remove", "sync"} {
		c := &xli.Command{Name: op, Brief: map[string]string{"ls": "Show host source and container destination mappings", "add": "Register or replace a mapping and publish current files", "remove": "Stop copying a mapping (existing destination files are retained)", "sync": "Publish current host files; applies on next agent start"}[op]}
		if op == "add" {
			dst := stringArg("DST", false)
			dst.Brief = "Container path; supports ${AGENT_CONFIG_DIR}, ${SESSION_HOME}, ${WORKSPACE}"
			c.Args = arg.Args{stringArg("SRC", false), dst}
		}
		if op == "remove" {
			c.Args = arg.Args{stringArg("DST", false)}
		}
		if op == "add" || op == "remove" {
			c.Flags = flg.Flags{stringFlag("agent", "Only claude or codex; omitted applies to both", "")}
		}
		c.Handler = onRun(func(ctx context.Context, c *xli.Command) error {
			root := stateFrom(ctx)
			if err := os.MkdirAll(root, 0700); err != nil {
				return err
			}
			lock, err := core.Lock(filepath.Join(root, "settings.lock"))
			if err != nil {
				return err
			}
			defer lock.Close()
			cfg, err := settings.Load(root)
			if err != nil {
				return err
			}
			if c.Name == "ls" {
				return writeOutput(c, cfg.Files)
			}
			if c.Name == "add" || c.Name == "remove" {
				dst := arg.MustGet[string](c, "DST")
				agent := flg.MustGet[string](c, "agent")
				if err := filemap.ValidateMappingDestination(dst, agent); err != nil {
					return err
				}
				dst = filepath.Clean(dst)
				files := make([]filemap.Mapping, 0, len(cfg.Files)+1)
				found := false
				for _, m := range cfg.Files {
					if m.Dst == dst && m.Agent == agent {
						found = true
						continue
					}
					files = append(files, m)
				}
				if c.Name == "add" {
					src := arg.MustGet[string](c, "SRC")
					if strings.HasPrefix(src, "~/") {
						home, err := os.UserHomeDir()
						if err != nil {
							return err
						}
						src = filepath.Join(home, src[2:])
					}
					src, err = filepath.Abs(src)
					if err != nil {
						return err
					}
					files = append(files, filemap.Mapping{Src: src, Dst: dst, Agent: agent})
				} else if !found {
					return fmt.Errorf("mapping not found for destination and agent")
				}
				cfg.Files = files
			}
			bundle, err := filemap.Snapshot(cfg.Files)
			if err != nil {
				return err
			}
			if err := settings.Save(root, cfg); err != nil {
				return err
			}
			out, err := publishFileMappings(ctx, root, bundle)
			if err != nil {
				return err
			}
			return writeOutput(c, out)
		})
		group.Commands = append(group.Commands, c)
	}
	return group
}

func publishFileMappings(ctx context.Context, root string, bundle filemap.Bundle) (*api.Receipt, error) {
	data, err := json.Marshal(bundle)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	conn, err := server.Dial(root)
	if err != nil {
		return nil, fmt.Errorf("mapping saved locally; publish with cxz config files sync after connecting to the manager: %w", err)
	}
	defer conn.Close()
	out, err := resourceclient.New(conn).FileMappings(ctx, &api.FileMappingsInput{Bundle: data})
	if status.Code(err) == codes.Unimplemented {
		return nil, fmt.Errorf("mapping saved locally; update cxz on the host, manager, and project runtimes, then run cxz config files sync")
	}
	if err != nil {
		return nil, fmt.Errorf("mapping saved locally; publishing failed (retry cxz config files sync): %w", err)
	}
	return out, nil
}
