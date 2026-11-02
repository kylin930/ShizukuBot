package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/dto/keyboard"
	"shizukubot/events"
)

var (
	ErrActiveSendUnavailable = errors.New("QQ active sending is unavailable under platform policy; reply to an authorized incoming event instead")
	ErrMissingReplyID        = errors.New("QQ passive reply requires a message ID or a supported event ID")
	ErrReplyLimit            = errors.New("QQ passive reply limit reached (5 attempts per incoming event)")
)

// replyState belongs to one admitted event. No Bot/global sequence map is used.
// Failed or ambiguous sends consume a sequence too: we never retry automatically.
type replyState struct {
	mu   sync.Mutex
	next uint32
}

func (s *replyState) allocate() (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next >= 5 {
		return 0, ErrReplyLimit
	}
	s.next++
	return s.next, nil
}

func replyIDs(e events.Event) (string, string) {
	switch e.Type {
	case "group_at_message", "group_message", "c2c_message", "at_message", "guild_message", "direct_message":
		return e.MessageID, ""
	}
	// Only documented management events with unambiguous targets are supported.
	switch e.PlatformType {
	case "FRIEND_ADD", "C2C_MSG_RECEIVE":
		if e.Source == events.C2C {
			return "", e.EventID
		}
	case "GROUP_ADD_ROBOT", "GROUP_MSG_RECEIVE":
		if e.Source == events.Group {
			return "", e.EventID
		}
	}
	return "", ""
}

func (b *Bot) reply(e events.Event) func(context.Context, events.Reply) error {
	return b.replyContext(b.requestContext(), e)
}
func (b *Bot) replyContext(life context.Context, e events.Event) func(context.Context, events.Reply) error {
	state := &replyState{}
	// Capture only routing/association data; do not retain private text or Raw.
	e.Message, e.Raw, e.Reply, e.PluginIDs = "", nil, nil, nil
	return func(ctx context.Context, r events.Reply) (err error) {
		call, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(life, cancel)
		defer stop()
		defer cancel()
		defer func() { b.recordSend(e, r.Kind, err) }()
		if err = life.Err(); err != nil {
			return err
		}
		if err = call.Err(); err != nil {
			return err
		}
		msgID, eventID := replyIDs(e)
		if e.Source == events.Guild || e.Source == events.Direct {
			if r.Kind != "text" {
				return errors.New("reply kind unsupported for this source")
			}
			if msgID == "" {
				return ErrMissingReplyID
			}
			m := &dto.MessageToCreate{Content: r.Text, MsgID: msgID}
			if e.Source == events.Guild {
				_, err = b.OpenAPI.PostMessage(call, e.ChannelID, m)
			} else {
				_, err = b.OpenAPI.PostDirectMessage(call, &dto.DirectMessage{GuildID: e.GuildID}, m)
			}
			return safeQQError(err)
		}
		if e.Source != events.Group && e.Source != events.C2C {
			return errors.New("reply source unsupported")
		}
		if msgID == "" && eventID == "" {
			return ErrMissingReplyID
		}
		target := e.UserID
		if e.Source == events.Group {
			target = e.GroupID
		}
		if target == "" {
			return errors.New("QQ reply target missing")
		}
		post := func(m dto.APIMessage) (*dto.Message, error) {
			if e.Source == events.Group {
				return b.OpenAPI.PostGroupMessage(call, target, m)
			}
			return b.OpenAPI.PostC2CMessage(call, target, m)
		}
		m := &dto.MessageToCreate{Content: r.Text, MsgType: dto.TextMsg, MsgID: msgID, EventID: eventID}
		switch r.Kind {
		case "text":
		case "markdown", "keyboard":
			m.Content = ""
			m.MsgType = dto.MarkdownMsg
			m.Markdown = &dto.Markdown{Content: r.Text}
			if r.Kind == "keyboard" {
				var kb keyboard.MessageKeyboard
				if json.Unmarshal([]byte(r.Keyboard), &kb) != nil {
					return errors.New("invalid QQ keyboard JSON")
				}
				m.Keyboard = &kb
			}
		case "image", "rich_media", "video":
			upload := &dto.RichMediaMessage{FileType: 1, URL: r.URL, SrvSendMsg: false}
			if r.Kind == "video" {
				upload.FileType = 2
			} else if data := parseDataURL(r.URL); data != "" {
				upload.URL = ""
				upload.FileData = data
			} else if strings.HasPrefix(r.URL, "data:") {
				return errors.New("invalid image data URL")
			}
			response, uploadErr := post(upload)
			if uploadErr != nil {
				return safeQQError(uploadErr)
			}
			if response == nil || len(response.FileInfo) == 0 {
				return errors.New("QQ upload response missing file_info")
			}
			m.MsgType = dto.RichMediaMsg
			m.Media = &dto.MediaInfo{FileInfo: response.FileInfo}
			if m.Content == "" {
				m.Content = " "
			} // documented media compatibility requirement
		default:
			return errors.New("reply kind unsupported for this source")
		}
		if err = call.Err(); err != nil {
			return err
		}
		// Allocate immediately before the final message POST, after upload/validation.
		m.MsgSeq, err = state.allocate()
		if err != nil {
			return err
		}
		_, err = post(m)
		return safeQQError(err)
	}
}

func parseDataURL(url string) string {
	if !strings.HasPrefix(url, "data:image/") {
		return ""
	}
	header, data, ok := strings.Cut(url, ",")
	if !ok || !strings.HasSuffix(header, ";base64") {
		return ""
	}
	if _, err := base64.StdEncoding.DecodeString(data); err != nil {
		return ""
	}
	return data
}

func (b *Bot) SendGroupMessage(groupID string, content string) error { return ErrActiveSendUnavailable }

func (b *Bot) SendGroupImage(groupID string, imageURL string) error { return ErrActiveSendUnavailable }

func (b *Bot) SendGroupImageData(groupID string, imageData []byte) error {
	return ErrActiveSendUnavailable
}

func (b *Bot) SendGroupRichMedia(groupID string, text, imageURL string) error {
	return ErrActiveSendUnavailable
}

func (b *Bot) SendGroupMarkdown(groupID string, content string) error {
	return ErrActiveSendUnavailable
}

func (b *Bot) SendGroupKeyboard(groupID string, markdown, keyboardJSON string) error {
	return ErrActiveSendUnavailable
}

func (b *Bot) SendGroupVideo(groupID string, videoURL string) error { return ErrActiveSendUnavailable }

func (b *Bot) SendC2CMessage(userID string, content string) error { return ErrActiveSendUnavailable }

func (b *Bot) SendC2CImage(userID string, imageURL string) error { return ErrActiveSendUnavailable }

func (b *Bot) SendC2CImageData(userID string, imageData []byte) error {
	return ErrActiveSendUnavailable
}

func (b *Bot) SendC2CRichMedia(userID string, text, imageURL string) error {
	return ErrActiveSendUnavailable
}

func (b *Bot) SendC2CMarkdown(userID string, content string) error { return ErrActiveSendUnavailable }

func (b *Bot) SendC2CKeyboard(userID string, markdown, keyboardJSON string) error {
	return ErrActiveSendUnavailable
}

func (b *Bot) SendC2CVideo(userID string, videoURL string) error { return ErrActiveSendUnavailable }
