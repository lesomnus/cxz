package main

import "github.com/lesomnus/xli"

// Keep help and completion groups consistent across platform-specific roots.
func categorizeCommands(root *xli.Command) {
	groups := []struct {
		name     string
		commands []string
	}{
		{"Resources", []string{"project", "session", "conversation", "account", "binding", "connection"}},
		{"Workspace", []string{"up", "down", "web", "expose", "config", "edit", "integration"}},
		{"Agents", []string{"ai", "mcp", "skill", "backend"}},
		{"Integration", []string{"devcontainer", "github", "gitconfig", "docker", "completion"}},
		{"Installation", []string{"install", "uninstall", "self-install", "self-update", "use", "manager", "purge", "version"}},
		{"Internal runtime", []string{"terminal-info"}},
	}
	var commands xli.Commands
	grouped := make(map[*xli.Command]bool)
	for _, group := range groups {
		for _, name := range group.commands {
			if command := root.Commands.Get(name); command != nil {
				command.Category = group.name
				commands = append(commands, command)
				grouped[command] = true
			}
		}
	}
	// Preserve internal entrypoints and any commands not yet categorized.
	for _, command := range root.Commands {
		if !grouped[command] {
			commands = append(commands, command)
		}
	}
	root.Commands = commands
}
