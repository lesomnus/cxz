package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/selfupdate"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/versionpin"
	"github.com/lesomnus/cxz/internal/versionuse"
	"io"
	"os"
	"time"
)

func prepareUseBackend(ctx context.Context, root string, p versionpin.Pin, out io.Writer) (*useBackend, error) {
	if _, e := transport.Load(root); os.IsNotExist(e) {
		return nil, nil
	} else if e != nil {
		return nil, e
	}
	if p.Image == "" {
		image, e := selfupdate.ManagerImage(ctx, p.Version)
		if e != nil {
			return nil, e
		}
		p.Image = image
	}
	tx, e := versionuse.Prepare(ctx, root, p, out)
	if e != nil {
		return nil, e
	}
	if tx == nil {
		return nil, nil
	}
	return &useBackend{pin: tx.Pin, apply: func(ctx context.Context) error { return versionuse.Apply(ctx, root, tx, out) }, finish: func(ctx context.Context) error { return versionuse.Finish(ctx, root, tx) }}, nil
}
func clearUseBackend(ctx context.Context, root string) error { return versionuse.Clear(ctx, root) }
func useInternal(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	switch args[0] {
	case "_use-policy":
		if len(args) != 3 {
			return true, fmt.Errorf("invalid use policy arguments")
		}
		var p versionpin.Pin
		if e := json.NewDecoder(io.LimitReader(os.Stdin, 65536)).Decode(&p); e != nil {
			return true, e
		}
		return true, versionuse.Policy(args[1], args[2], p)
	case "_use-project":
		if len(args) != 5 {
			return true, fmt.Errorf("invalid use project arguments")
		}
		root, generation, version := args[2], args[3], args[4]
		if len(generation) != 24 {
			return true, fmt.Errorf("invalid transaction ID")
		}
		for _, r := range generation {
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
				return true, fmt.Errorf("invalid transaction ID")
			}
		}
		switch args[1] {
		case "validate":
			return true, versionuse.ValidateProject(root)
		case "stop":
			return true, versionuse.StopProject(ctx, root, generation, version)
		case "resume":
			return true, versionuse.ResumeProject(ctx, root, generation, version)
		}
		return true, fmt.Errorf("invalid use project action")
	}
	return false, nil
}
