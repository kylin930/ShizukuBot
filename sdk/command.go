package sdk

import (
	"context"
	"unicode"
)

// CommandHandler processes a parsed command. Errors propagate to the Manager.
// An alias preserves the original Host method signatures for existing Go plugins.
type CommandHandler = func(context.Context, Call) error

// EventHandler processes a canonical event with the same signature as a command.
type EventHandler = func(context.Context, Call) error

// Registrar registers callbacks during initialization only. Registration errors
// must be returned; the simple implementation also remembers ignored failures.
type Registrar interface {
	Command(string, CommandHandler) error
	On(EventType, EventHandler) error
}

// ParseCommand uses a literal slash prefix and shell-like quoted arguments, without expansion.
func ParseCommand(message string) (string, []string, bool) {
	fields := []string{}
	word := ""
	var quote rune
	escaped, started := false, false
	for _, r := range message {
		if escaped {
			word += string(r)
			escaped = false
			started = true
			continue
		}
		if r == '\\' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word += string(r)
			}
			started = true
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			if started {
				fields = append(fields, word)
				word = ""
				started = false
			}
			continue
		}
		word += string(r)
		started = true
	}
	if quote != 0 || escaped {
		return "", nil, false
	}
	if started {
		fields = append(fields, word)
	}
	if len(fields) == 0 || len(fields[0]) < 2 || fields[0][0] != '/' {
		return "", nil, false
	}
	name := fields[0][1:]
	if !ValidID(name) {
		return "", nil, false
	}
	return name, fields[1:], true
}
