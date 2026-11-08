package sdk

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestReplyAtCarriesMention(t *testing.T) {
	var got Reply
	c := Call{Event: Event{Reply: func(_ context.Context, r Reply) error { got = r; return nil }}}
	if err := c.ReplyAt(context.Background(), "你好", "member"); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "text" || got.Text != "你好" || got.Mention != "member" {
		t.Fatalf("unexpected reply: %+v", got)
	}
}

func TestReplyImageBytesBuildsDataURL(t *testing.T) {
	var got Reply
	c := Call{Event: Event{Reply: func(_ context.Context, r Reply) error { got = r; return nil }}}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if err := c.ReplyImageBytes(context.Background(), png); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "image" || !strings.HasPrefix(got.URL, "data:image/png;base64,") {
		t.Fatalf("unexpected image reply: %+v", got)
	}
	if err := c.ReplyImageBytes(context.Background(), []byte("not an image")); err == nil {
		t.Fatal("non-image bytes were accepted")
	}
}

func TestReplyVoiceBytesBuildsDataURL(t *testing.T) {
	var got Reply
	c := Call{Event: Event{Reply: func(_ context.Context, r Reply) error { got = r; return nil }}}
	wav := []byte("RIFF\x00\x00\x00\x00WAVEfmt ")
	if err := c.ReplyVoiceBytes(context.Background(), wav, "audio/wav"); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "voice" || !strings.HasPrefix(got.URL, "data:audio/wav;base64,") {
		t.Fatalf("unexpected voice reply: %+v", got)
	}
	if err := c.ReplyVoiceBytes(context.Background(), nil, "audio/wav"); err == nil {
		t.Fatal("empty voice data was accepted")
	}
	if err := c.ReplyVoiceBytes(context.Background(), wav, "text/plain"); err == nil {
		t.Fatal("non-audio MIME was accepted")
	}
}

func TestActionHelpers(t *testing.T) {
	var actions []Action
	c := Call{Event: Event{MessageID: "msg", UserID: "user",
		Act: func(_ context.Context, a Action) error { actions = append(actions, a); return nil }}}
	ctx := context.Background()
	if err := c.Recall(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Mute(ctx, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := c.Unmute(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Kick(ctx, true); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 4 {
		t.Fatalf("got %d actions", len(actions))
	}
	if actions[0] != (Action{Kind: ActionRecall, MessageID: "msg"}) {
		t.Fatalf("unexpected recall: %+v", actions[0])
	}
	if actions[1] != (Action{Kind: ActionMute, UserID: "user", Seconds: 600}) {
		t.Fatalf("unexpected mute: %+v", actions[1])
	}
	if actions[2] != (Action{Kind: ActionUnmute, UserID: "user"}) {
		t.Fatalf("unexpected unmute: %+v", actions[2])
	}
	if actions[3] != (Action{Kind: ActionKick, UserID: "user", Blacklist: true}) {
		t.Fatalf("unexpected kick: %+v", actions[3])
	}
}

func TestMentionUserIDsAndCommandText(t *testing.T) {
	e := Event{Message: `禁言 <qqbot-at-user id="MEMBER" /> 10分钟 <@ABC123>`}
	ids := MentionUserIDs(e)
	if len(ids) != 2 || ids[0] != "MEMBER" || ids[1] != "ABC123" {
		t.Fatalf("unexpected mention IDs: %v", ids)
	}
	if got := Text(e); got != "禁言  10分钟" {
		t.Fatalf("unexpected command text: %q", got)
	}
}

func TestActionRequiresHostCapability(t *testing.T) {
	c := Call{Event: Event{UserID: "user"}}
	if err := c.Mute(context.Background(), time.Minute); err == nil {
		t.Fatal("mute succeeded without host capability")
	}
	if err := c.Mute(context.Background(), 0); err == nil {
		t.Fatal("zero mute duration accepted")
	}
}
