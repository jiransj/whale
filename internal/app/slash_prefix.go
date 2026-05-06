package app

import appcommands "github.com/usewhale/whale/internal/app/commands"

func expandUniqueSlashPrefix(line string) string {
	return appcommands.ExpandUniqueSlashPrefix(line, CommandsHelp, "/tools", "/tool", "/budget", "/approval", "/thinking")
}

func parseSlashCommands(help string) []string {
	return appcommands.ParseSlashCommands(help)
}
