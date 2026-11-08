package sdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Keyboard is a QQ message keyboard sent alongside a Markdown reply. QQ groups
// and C2C chats accept a custom keyboard (Content); guilds accept a keyboard
// template ID. Both require platform-side application permission.
type Keyboard struct {
	ID      string           `json:"id,omitempty"`
	Content *KeyboardContent `json:"content,omitempty"`
}

// KeyboardContent is the custom keyboard layout. QQ shows at most five rows and
// five buttons per row.
type KeyboardContent struct {
	Rows  []KeyboardRow  `json:"rows,omitempty"`
	Style *KeyboardStyle `json:"style,omitempty"`
}

// KeyboardStyle customizes the keyboard appearance.
type KeyboardStyle struct {
	FontSize string `json:"font_size,omitempty"`
}

// KeyboardRow is one horizontal row of buttons.
type KeyboardRow struct {
	Buttons []Button `json:"buttons,omitempty"`
}

// Button is one clickable message button.
type Button struct {
	ID         string       `json:"id,omitempty"`
	RenderData ButtonRender `json:"render_data"`
	Action     ButtonAction `json:"action"`
	// GroupID marks buttons that disable each other after one is clicked.
	GroupID string `json:"group_id,omitempty"`
}

// ButtonRender is the visible part of a button.
type ButtonRender struct {
	Label        string `json:"label"`
	VisitedLabel string `json:"visited_label,omitempty"`
	// Style accepts 0 (gray outline), 1 (blue outline), 3 (red text on white)
	// and 4 (white text on blue).
	Style int `json:"style,omitempty"`
}

// ButtonActionType is the QQ button action type.
type ButtonActionType uint32

const (
	// ButtonURLAction opens a URL or mini-program schema.
	ButtonURLAction ButtonActionType = 0
	// ButtonCallbackAction invokes the application's interaction callback with Data.
	ButtonCallbackAction ButtonActionType = 1
	// ButtonCommandAction prefills ("@bot Data") in the input box.
	ButtonCommandAction ButtonActionType = 2
	// ButtonMQQAPIAction opens a client native scheme.
	ButtonMQQAPIAction ButtonActionType = 3
)

// ButtonAction is what happens when a button is clicked.
type ButtonAction struct {
	Type ButtonActionType `json:"type"`
	// Data is a URL, callback payload or command, depending on Type.
	Data       string `json:"data,omitempty"`
	Enter      bool   `json:"enter"`
	ClickLimit uint32 `json:"click_limit,omitempty"`
	// AtBotShowChannelList lets guild users pick a channel before the bot is @-ed.
	AtBotShowChannelList bool         `json:"at_bot_show_channel_list,omitempty"`
	Modal                *ButtonModal `json:"modal,omitempty"`
}

// ButtonModal asks for confirmation before the action runs.
type ButtonModal struct {
	Content     string `json:"content,omitempty"`
	ConfirmText string `json:"confirm_text,omitempty"`
	CancelText  string `json:"cancel_text,omitempty"`
}

// NewKeyboard builds a custom keyboard from rows.
func NewKeyboard(rows ...KeyboardRow) Keyboard {
	return Keyboard{Content: &KeyboardContent{Rows: rows}}
}

// NewRow groups buttons into one row.
func NewRow(buttons ...Button) KeyboardRow {
	return KeyboardRow{Buttons: buttons}
}

// CommandButton sends data to the bot as a command when clicked. The input box
// receives "@bot <command>" and Enter submits it immediately.
func CommandButton(id, label, command string) Button {
	return Button{
		ID:         id,
		RenderData: ButtonRender{Label: label, VisitedLabel: label},
		Action:     ButtonAction{Type: ButtonCommandAction, Data: command, Enter: true},
	}
}

// CallbackButton reports the click to the application's interaction callback.
func CallbackButton(id, label, data string) Button {
	return Button{
		ID:         id,
		RenderData: ButtonRender{Label: label, VisitedLabel: label},
		Action:     ButtonAction{Type: ButtonCallbackAction, Data: data},
	}
}

// LinkButton opens a URL when clicked. URL buttons are not available in every
// QQ conversation type; send failures are returned to the caller.
func LinkButton(id, label, url string) Button {
	return Button{
		ID:         id,
		RenderData: ButtonRender{Label: label, VisitedLabel: label},
		Action:     ButtonAction{Type: ButtonURLAction, Data: url},
	}
}

// Validate checks the documented QQ keyboard limits before the payload leaves
// the process.
func (k Keyboard) Validate() error {
	switch {
	case k.ID != "" && k.Content != nil:
		return errors.New("keyboard accepts either a template ID or custom content, not both")
	case k.ID == "" && k.Content == nil:
		return errors.New("keyboard requires a template ID or custom content")
	case k.ID != "":
		return nil
	}
	rows := k.Content.Rows
	if len(rows) == 0 || len(rows) > 5 {
		return fmt.Errorf("keyboard needs 1-5 rows, got %d", len(rows))
	}
	for rowIndex, row := range rows {
		if len(row.Buttons) == 0 || len(row.Buttons) > 5 {
			return fmt.Errorf("keyboard row %d needs 1-5 buttons, got %d", rowIndex+1, len(row.Buttons))
		}
		for buttonIndex, button := range row.Buttons {
			if err := button.validate(); err != nil {
				return fmt.Errorf("keyboard row %d button %d: %w", rowIndex+1, buttonIndex+1, err)
			}
		}
	}
	return nil
}

func (b Button) validate() error {
	label := strings.TrimSpace(b.RenderData.Label)
	if label == "" {
		return errors.New("label required")
	}
	if utf8.RuneCountInString(label) > 20 {
		return errors.New("label exceeds 20 characters")
	}
	if utf8.RuneCountInString(b.RenderData.VisitedLabel) > 20 {
		return errors.New("visited label exceeds 20 characters")
	}
	switch b.RenderData.Style {
	case 0, 1, 3, 4:
	default:
		return errors.New("style must be 0, 1, 3 or 4")
	}
	switch b.Action.Type {
	case ButtonURLAction, ButtonCallbackAction, ButtonCommandAction, ButtonMQQAPIAction:
	default:
		return errors.New("action type must be 0-3")
	}
	if strings.TrimSpace(b.Action.Data) == "" {
		return errors.New("action data required")
	}
	if len(b.Action.Data) > 1024 {
		return errors.New("action data exceeds 1024 bytes")
	}
	if modal := b.Action.Modal; modal != nil {
		if content := strings.TrimSpace(modal.Content); content == "" {
			return errors.New("modal content required")
		} else if utf8.RuneCountInString(content) > 40 {
			return errors.New("modal content exceeds 40 characters")
		}
		if utf8.RuneCountInString(modal.ConfirmText) > 4 || utf8.RuneCountInString(modal.CancelText) > 4 {
			return errors.New("modal confirm/cancel text exceeds 4 characters")
		}
	}
	return nil
}

// JSON validates the keyboard and returns its wire representation.
func (k Keyboard) JSON() (string, error) {
	if err := k.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(k)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
