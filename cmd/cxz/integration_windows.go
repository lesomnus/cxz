package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"golang.org/x/sys/windows"
)

const terminalProfileGUID = "{93ed3c79-b2a5-4e55-a831-5cb24c626bb8}"

type terminalFragment struct {
	Profiles []terminalProfile `json:"profiles"`
}

type terminalProfile struct {
	GUID              string `json:"guid"`
	Name              string `json:"name"`
	Commandline       string `json:"commandline"`
	StartingDirectory string `json:"startingDirectory"`
}

func integrationCommand() *xli.Command {
	group := &xli.Command{Name: "integration", Brief: "Manage desktop application integrations", Handler: xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error {
		return c.PrintHelp(c.Writer)
	})}
	for _, verb := range []string{"add", "remove", "ls"} {
		command := &xli.Command{Name: verb, Brief: map[string]string{"add": "Register or refresh an integration", "remove": "Remove an integration", "ls": "Show integration status"}[verb]}
		if verb != "ls" {
			command.Args = arg.Args{&arg.String{Name: "INTEGRATION"}}
		}
		command.Handler = xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error {
			if verb != "ls" && arg.MustGet[string](c, "INTEGRATION") != "windows-terminal" {
				return fmt.Errorf("unknown integration %q; available: windows-terminal", arg.MustGet[string](c, "INTEGRATION"))
			}
			local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, windows.KF_FLAG_DONT_VERIFY)
			if err != nil {
				return err
			}
			path := filepath.Join(local, "Microsoft", "Windows Terminal", "Fragments", "cxz", "cxz.json")
			switch verb {
			case "add":
				executable, err := os.Executable()
				if err != nil {
					return err
				}
				root := c
				for root.HasParent() {
					root = root.Parent()
				}
				state, err := filepath.Abs(flg.MustGet[string](root, "state"))
				if err != nil {
					return err
				}
				if err := writeTerminalFragment(path, executable, state); err != nil {
					return err
				}
				fmt.Fprintf(c.Writer, "Registered Windows Terminal profile: cxz\nExecutable: %s\nFragment: %s\nReopen Windows Terminal to select the cxz profile.\n", executable, path)
			case "remove":
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return err
				}
				fmt.Fprintf(c.Writer, "Removed cxz Windows Terminal profile: %s\n", path)
			case "ls":
				data, err := os.ReadFile(path)
				if os.IsNotExist(err) {
					fmt.Fprintf(c.Writer, "windows-terminal  not registered\nFragment: %s\n", path)
					return nil
				}
				if err != nil {
					return err
				}
				var fragment terminalFragment
				if err := json.Unmarshal(data, &fragment); err != nil {
					return fmt.Errorf("invalid cxz Windows Terminal fragment %s: %w", path, err)
				}
				for _, profile := range fragment.Profiles {
					if profile.GUID == terminalProfileGUID {
						fmt.Fprintf(c.Writer, "windows-terminal  registered\nCommand: %s\nFragment: %s\n", profile.Commandline, path)
						return nil
					}
				}
				return fmt.Errorf("cxz profile is missing from %s; rerun integration add windows-terminal", path)
			}
			return nil
		})
		group.Commands = append(group.Commands, command)
	}
	return group
}

func writeTerminalFragment(path, executable, state string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return core.WriteJSON(path, terminalFragment{Profiles: []terminalProfile{{
		GUID: terminalProfileGUID, Name: "cxz",
		Commandline:       windows.ComposeCommandLine([]string{executable, "--state", state}),
		StartingDirectory: "%USERPROFILE%",
	}}})
}
