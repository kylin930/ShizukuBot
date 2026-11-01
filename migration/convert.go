// Package migration reads historical data offline; it is never an event execution engine.
package migration

import (
	"encoding/json"
	"errors"
	"fmt"
	"shizukubot/plugins"
	"sort"
	"strings"
)

type legacy struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	BotID       string          `json:"bot_id"`
	Graph       json.RawMessage `json:"graph"`
	CustomNodes json.RawMessage `json:"custom_nodes"`
}
type node struct {
	ID         int            `json:"id"`
	Type       string         `json:"type"`
	Mode       int            `json:"mode"`
	Properties map[string]any `json:"properties"`
	Inputs     []port         `json:"inputs"`
	Outputs    []port         `json:"outputs"`
}
type port struct {
	Name  string `json:"name"`
	Type  any    `json:"type"`
	Link  int    `json:"link"`
	Links []int  `json:"links"`
}
type link struct {
	ID         int `json:"id"`
	Origin     int `json:"origin_id"`
	OriginSlot int `json:"origin_slot"`
	Target     int `json:"target_id"`
	TargetSlot int `json:"target_slot"`
}

func graph(data json.RawMessage) ([]node, []link, error) {
	var raw struct {
		Nodes []node            `json:"nodes"`
		Links []json.RawMessage `json:"links"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}
	links := []link{}
	for _, b := range raw.Links {
		var arr []any
		if json.Unmarshal(b, &arr) == nil {
			if len(arr) != 6 {
				return nil, nil, errors.New("invalid link array")
			}
			ints := make([]int, 5)
			for i := range ints {
				n, ok := arr[i].(float64)
				if !ok || n != float64(int(n)) {
					return nil, nil, errors.New("invalid link number")
				}
				ints[i] = int(n)
			}
			links = append(links, link{ints[0], ints[1], ints[2], ints[3], ints[4]})
		} else {
			var l link
			if json.Unmarshal(b, &l) != nil {
				return nil, nil, errors.New("invalid link")
			}
			links = append(links, l)
		}
	}
	return raw.Nodes, links, nil
}
func capabilities(nodes []node) []string {
	seen := map[string]bool{}
	for _, n := range nodes {
		seen[n.Type] = true
	}
	out := []string{}
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// convert accepts only a fully accounted-for linear trigger / optional condition / literal reply.
func convert(old legacy, id string) (plugins.Manifest, string, []string, error) {
	mf := plugins.Manifest{APIVersion: 1, ID: id, Name: old.Name, Description: old.Description, Version: "1.0.0", Runtime: "javascript", Entry: "dist/index.js", Permissions: []string{"events.subscribe", "message.send"}, ConfigSchema: plugins.Schema{Type: "object"}}
	if mf.Name == "" {
		mf.Name = id
	}
	nodes, links, err := graph(old.Graph)
	caps := capabilities(nodes)
	if err != nil {
		return mf, "", caps, err
	}
	if string(old.CustomNodes) != "" && string(old.CustomNodes) != "null" && string(old.CustomNodes) != "[]" {
		return mf, "", caps, errors.New("embedded custom nodes require manual migration")
	}
	if len(nodes) < 2 || len(nodes) > 3 || len(links) != len(nodes)-1 {
		return mf, "", caps, errors.New("only a complete single linear reply path is proven equivalent")
	}
	byID := map[int]node{}
	var event, action, condition node
	for _, n := range nodes {
		if _, exists := byID[n.ID]; exists || n.Mode != 0 {
			return mf, "", caps, errors.New("duplicate ID or non-default node mode")
		}
		byID[n.ID] = n
		switch n.Type {
		case "event/on_group_message", "event/on_c2c_message", "event/on_command":
			if event.Type != "" {
				return mf, "", caps, errors.New("multiple event entries")
			}
			event = n
		case "action/send_message":
			if action.Type != "" {
				return mf, "", caps, errors.New("multiple actions")
			}
			action = n
		case "condition/text_contains", "condition/text_equals":
			condition = n
		default:
			return mf, "", caps, fmt.Errorf("unsupported node %s", n.Type)
		}
	}
	if event.Type == "" || action.Type == "" || len(event.Inputs) > 0 {
		return mf, "", caps, errors.New("missing event or action")
	}
	chain := []node{event, action}
	if condition.Type != "" {
		chain = []node{event, condition, action}
	}
	expected := map[int]bool{}
	for i, n := range chain {
		if i == len(chain)-1 {
			for _, out := range n.Outputs {
				if len(out.Links) > 0 {
					return mf, "", caps, errors.New("action has additional branches")
				}
			}
			continue
		}
		var out port
		count := 0
		for _, p := range n.Outputs {
			if len(p.Links) > 0 {
				out = p
				count++
			}
		}
		if count != 1 || len(out.Links) != 1 {
			return mf, "", caps, errors.New("branching or disconnected output")
		}
		if i == 0 && (out.Name != "触发" || fmt.Sprint(out.Type) != "-1") {
			return mf, "", caps, errors.New("event data outputs are not unconditional trigger ports")
		}
		if n.Type == condition.Type && out.Name != "结果" {
			return mf, "", caps, errors.New("condition output is not its true result")
		}
		next := chain[i+1]
		found := false
		for _, l := range links {
			if l.ID == out.Links[0] {
				if expected[l.ID] || l.Origin != n.ID || l.Target != next.ID || l.OriginSlot < 0 || l.OriginSlot >= len(n.Outputs) || n.Outputs[l.OriginSlot].Name != out.Name || l.TargetSlot < 0 || l.TargetSlot >= len(next.Inputs) {
					return mf, "", caps, errors.New("inconsistent connection")
				}
				input := next.Inputs[l.TargetSlot]
				if input.Link != l.ID || fmt.Sprint(input.Type) != "-1" {
					return mf, "", caps, errors.New("non-trigger data dependencies require manual migration")
				}
				expected[l.ID] = true
				found = true
			}
		}
		if !found {
			return mf, "", caps, errors.New("dangling link")
		}
	}
	if len(expected) != len(links) {
		return mf, "", caps, errors.New("unaccounted links")
	}
	for _, n := range chain[1:] {
		if len(n.Inputs) != 1 {
			return mf, "", caps, errors.New("extra data inputs")
		}
	}
	text, ok := action.Properties["template"].(string)
	if !ok || text == "" {
		return mf, "", caps, errors.New("literal reply template required")
	}
	predicate := "true"
	types := []string{"group_at_message", "group_message"}
	switch event.Type {
	case "event/on_c2c_message":
		types = []string{"c2c_message"}
	case "event/on_command":
		types = []string{"*"}
		prefix, _ := event.Properties["prefix"].(string)
		if prefix == "" {
			prefix = "/"
		}
		command, _ := event.Properties["command"].(string)
		predicate = "legacyCommand(ctx," + jsQuote(prefix) + "," + jsQuote(command) + ")"
	}
	if condition.Type != "" {
		key := "expected"
		if condition.Type == "condition/text_contains" {
			key = "keyword"
		}
		value, ok := condition.Properties[key].(string)
		if !ok {
			return mf, "", caps, errors.New("condition value must be a literal string")
		}
		test := "ctx.message === " + jsQuote(value)
		if condition.Type == "condition/text_contains" {
			test = fmt.Sprintf("%t && ctx.message.includes(%s)", value != "", jsQuote(value))
		}
		predicate = "(" + predicate + ") && (" + test + ")"
	}
	// Commands historically stripped bot mentions and used whitespace tokenization,
	// not shell argument parsing. Keep that matcher inside generated ordinary code.
	helper := `function legacyCommand(ctx,prefix,name){let text=ctx.message;const ids=ctx.botMentionIds??[];if(ids.length){for(const id of ids){text=text.split('<@'+id+'>').join('').split('<@!'+id+'>').join('');}}else{text=text.replace(/<@!?[0-9A-Fa-f]+>/g,'');}text=text.replace(/^[ \u00a0]+|[ \u00a0]+$/g,'');if(!text.startsWith(prefix))return false;const parts=text.slice(prefix.length).replace(/^\p{White_Space}+|\p{White_Space}+$/gu,'').split(/\p{White_Space}+/u);return name==='' || (parts[0]!=='' && parts[0]===name);}`
	entries := []string{}
	for _, typ := range types {
		entries = append(entries, jsQuote(typ)+":respond")
	}
	code := fmt.Sprintf("// Generated from an audited linear legacy reply; review before trusting.\n%s\nasync function respond(ctx){if(%s && (ctx.groupId || ctx.userId))await ctx.reply(%s);}\nexport default {id:%s,version:'1.0.0',events:{%s}};\n", helper, predicate, jsQuote(text), jsQuote(id), strings.Join(entries, ","))
	return mf, code, caps, nil
}

func jsQuote(value string) string { b, _ := json.Marshal(value); return string(b) }
