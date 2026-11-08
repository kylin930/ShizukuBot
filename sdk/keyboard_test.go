package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestKeyboardBuildsCommandButtons(t *testing.T) {
	kb := NewKeyboard(
		NewRow(CommandButton("help", "查看帮助", "#王者帮助"), CommandButton("query", "查询战绩", "#查询战绩")),
		NewRow(LinkButton("docs", "使用说明", "https://example.com/help")),
	)
	raw, err := kb.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Content struct {
			Rows []struct {
				Buttons []struct {
					ID         string `json:"id"`
					RenderData struct {
						Label        string `json:"label"`
						VisitedLabel string `json:"visited_label"`
					} `json:"render_data"`
					Action struct {
						Type  int    `json:"type"`
						Data  string `json:"data"`
						Enter bool   `json:"enter"`
					} `json:"action"`
				} `json:"buttons"`
			} `json:"rows"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Content.Rows) != 2 || len(parsed.Content.Rows[0].Buttons) != 2 {
		t.Fatalf("unexpected layout: %s", raw)
	}
	help := parsed.Content.Rows[0].Buttons[0]
	if help.ID != "help" || help.RenderData.Label != "查看帮助" || help.RenderData.VisitedLabel != "查看帮助" {
		t.Fatalf("unexpected render data: %+v", help)
	}
	if help.Action.Type != 2 || help.Action.Data != "#王者帮助" || !help.Action.Enter {
		t.Fatalf("unexpected command action: %+v", help.Action)
	}
	link := parsed.Content.Rows[1].Buttons[0].Action
	if link.Type != 0 || link.Data != "https://example.com/help" || link.Enter {
		t.Fatalf("unexpected link action: %+v", link)
	}
}

func TestKeyboardValidate(t *testing.T) {
	valid := func() Keyboard {
		return NewKeyboard(NewRow(CommandButton("a", "A", "a")))
	}
	cases := []struct {
		name string
		kb   Keyboard
	}{
		{"empty", Keyboard{}},
		{"id and content", func() Keyboard {
			kb := valid()
			kb.ID = "template"
			return kb
		}()},
		{"no rows", NewKeyboard()},
		{"too many rows", NewKeyboard(NewRow(CommandButton("a", "A", "a")), NewRow(CommandButton("b", "B", "b")),
			NewRow(CommandButton("c", "C", "c")), NewRow(CommandButton("d", "D", "d")),
			NewRow(CommandButton("e", "E", "e")), NewRow(CommandButton("f", "F", "f")))},
		{"empty row", NewKeyboard(NewRow())},
		{"long label", NewKeyboard(NewRow(CommandButton("a", strings.Repeat("按", 21), "a")))},
		{"bad style", func() Keyboard {
			button := CommandButton("a", "A", "a")
			button.RenderData.Style = 2
			return NewKeyboard(NewRow(button))
		}()},
		{"bad action type", func() Keyboard {
			button := CommandButton("a", "A", "a")
			button.Action.Type = 9
			return NewKeyboard(NewRow(button))
		}()},
		{"empty data", NewKeyboard(NewRow(CommandButton("a", "A", "")))},
		{"long data", NewKeyboard(NewRow(CommandButton("a", "A", strings.Repeat("x", 1025))))},
		{"long modal", func() Keyboard {
			button := CommandButton("a", "A", "a")
			button.Action.Modal = &ButtonModal{Content: strings.Repeat("确", 41)}
			return NewKeyboard(NewRow(button))
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.kb.Validate(); err == nil {
				t.Fatal("invalid keyboard accepted")
			}
		})
	}
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Keyboard{ID: "template"}).Validate(); err != nil {
		t.Fatalf("template keyboard rejected: %v", err)
	}
}

func TestReplyKeyboardCarriesMarkdownAndPayload(t *testing.T) {
	var got Reply
	c := Call{Event: Event{Reply: func(_ context.Context, r Reply) error { got = r; return nil }}}
	kb := NewKeyboard(NewRow(CommandButton("query", "查询战绩", "#查询战绩")))
	if err := c.ReplyKeyboard(context.Background(), "**还没有绑定**", kb); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "keyboard" || got.Text != "**还没有绑定**" || got.Keyboard == "" {
		t.Fatalf("unexpected reply: %+v", got)
	}
	if !strings.Contains(got.Keyboard, `"data":"#查询战绩"`) {
		t.Fatalf("unexpected keyboard payload: %s", got.Keyboard)
	}
	if err := c.ReplyKeyboard(context.Background(), "", kb); err == nil {
		t.Fatal("empty markdown accepted")
	}
	if err := c.ReplyKeyboard(context.Background(), "text", Keyboard{}); err == nil {
		t.Fatal("invalid keyboard accepted")
	}
}

func TestReplyKeyboardJSONAcceptsTemplate(t *testing.T) {
	var got Reply
	c := Call{Event: Event{Reply: func(_ context.Context, r Reply) error { got = r; return nil }}}
	if err := c.ReplyKeyboardJSON(context.Background(), "模板消息", `{"id":"template"}`); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "keyboard" || got.Keyboard != `{"id":"template"}` {
		t.Fatalf("unexpected reply: %+v", got)
	}
	if err := c.ReplyKeyboardJSON(context.Background(), "模板消息", "not json"); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if err := c.ReplyKeyboardJSON(context.Background(), "模板消息", `{}`); err == nil {
		t.Fatal("empty keyboard accepted")
	}
}

func TestReplyKeyboardRequiresHostCapability(t *testing.T) {
	c := Call{}
	kb := NewKeyboard(NewRow(CommandButton("a", "A", "a")))
	if err := c.ReplyKeyboard(context.Background(), "text", kb); err == nil {
		t.Fatal("keyboard reply succeeded without host capability")
	}
}

func TestReplyKeyboardOrTextFallsBack(t *testing.T) {
	var kinds []string
	c := Call{Event: Event{Reply: func(_ context.Context, r Reply) error {
		kinds = append(kinds, r.Kind)
		if r.Kind == "keyboard" {
			return errors.New("keyboard not allowed")
		}
		return nil
	}}}
	kb := NewKeyboard(NewRow(CommandButton("a", "A", "a")))
	if err := c.ReplyKeyboardOrText(context.Background(), "纯文本", kb); err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 2 || kinds[0] != "keyboard" || kinds[1] != "text" {
		t.Fatalf("unexpected sends: %v", kinds)
	}
}
