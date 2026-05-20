package unipile

import (
	"context"
	"errors"
	"strings"
	"time"
)

// SendInvitationParams configures a connection-invitation send.
type SendInvitationParams struct {
	AccountID  string // our local account id (Unipile account_id)
	ProviderID string // LinkedIn provider_id of the prospect
	Message    string // optional note; empty = no note
	NoteLimit  int    // truncation cap; 0 → 300 free / 1900 premium decided by caller
}

// SendInvitationResult is the success payload of SendInvitation.
type SendInvitationResult struct {
	InvitationID string `json:"invitation_id"`
	DryRun       bool   `json:"dry_run,omitempty"`
}

// SendInvitation sends a LinkedIn connection invitation. If the client is in
// dry-run mode, returns a stub result and never hits the wire.
//
// Endpoint: POST /api/v1/users/invite  (verify against Unipile docs)
func (c *Client) SendInvitation(ctx context.Context, p SendInvitationParams) (*SendInvitationResult, error) {
	if p.AccountID == "" {
		return nil, errors.New("unipile: send_invitation: account_id required")
	}
	if p.ProviderID == "" {
		return nil, errors.New("unipile: send_invitation: provider_id required")
	}

	if c.dryRun {
		return &SendInvitationResult{InvitationID: "dry-run-" + time.Now().Format("20060102T150405"), DryRun: true}, nil
	}

	note := p.Message
	if p.NoteLimit > 0 && len(note) > p.NoteLimit {
		note = note[:p.NoteLimit]
	}

	body := map[string]any{
		"account_id":  p.AccountID,
		"provider_id": p.ProviderID,
	}
	if note != "" {
		body["message"] = note
	}

	var out SendInvitationResult
	if err := c.do(ctx, "POST", "/api/v1/users/invite", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SendMessageParams configures a message send into an existing chat.
type SendMessageParams struct {
	ChatID string
	Text   string
}

// SendMessageResult is the success payload of SendMessage.
type SendMessageResult struct {
	MessageID string `json:"message_id"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// SendMessage sends a text message into an existing chat.
//
// Endpoint: POST /api/v1/chats/{chat_id}/messages  (verify against Unipile docs)
func (c *Client) SendMessage(ctx context.Context, p SendMessageParams) (*SendMessageResult, error) {
	if p.ChatID == "" {
		return nil, errors.New("unipile: send_message: chat_id required")
	}
	if strings.TrimSpace(p.Text) == "" {
		return nil, errors.New("unipile: send_message: text required")
	}
	if c.dryRun {
		return &SendMessageResult{MessageID: "dry-run-" + time.Now().Format("20060102T150405"), DryRun: true}, nil
	}

	body := map[string]any{"text": p.Text}
	var out SendMessageResult
	if err := c.do(ctx, "POST", "/api/v1/chats/"+p.ChatID+"/messages", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StartNewChatParams configures the creation of a new chat with one or more
// attendees by their provider_ids. This is the path used when we don't have a
// chat_id yet (1st degree, never messaged).
type StartNewChatParams struct {
	AccountID    string
	AttendeesIDs []string // provider_ids
	Text         string
}

// StartNewChatResult is the success payload of StartNewChat.
type StartNewChatResult struct {
	ChatID    string `json:"chat_id"`
	MessageID string `json:"message_id"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// StartNewChat opens a new chat by sending the first message.
//
// Endpoint: POST /api/v1/chats  (multipart/form-data — verify against docs)
func (c *Client) StartNewChat(ctx context.Context, p StartNewChatParams) (*StartNewChatResult, error) {
	if p.AccountID == "" {
		return nil, errors.New("unipile: start_chat: account_id required")
	}
	if len(p.AttendeesIDs) == 0 {
		return nil, errors.New("unipile: start_chat: at least one attendee required")
	}
	if strings.TrimSpace(p.Text) == "" {
		return nil, errors.New("unipile: start_chat: text required")
	}
	if c.dryRun {
		stamp := time.Now().Format("20060102T150405")
		return &StartNewChatResult{ChatID: "dry-chat-" + stamp, MessageID: "dry-msg-" + stamp, DryRun: true}, nil
	}

	fields := map[string]string{
		"account_id":    p.AccountID,
		"text":          p.Text,
		"attendees_ids": strings.Join(p.AttendeesIDs, ","),
	}
	var out StartNewChatResult
	if err := c.doMultipart(ctx, "/api/v1/chats", fields, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---- Action stubs (implement when Unipile docs / live account available) ----
// All return ErrNotImplemented for now so callers fail loudly instead of
// silently no-op'ing. The signatures are stable; only the bodies are TODO.

// ErrNotImplemented is returned by action stubs that haven't been wired yet.
var ErrNotImplemented = errors.New("unipile: action not implemented yet")

// VisitProfile triggers a profile view as the account.
// TODO: implement against POST /api/v1/users/{provider_id}/profile/view  (verify path).
func (c *Client) VisitProfile(ctx context.Context, accountID, providerID string) error {
	if c.dryRun {
		return nil
	}
	return ErrNotImplemented
}

// Follow follows a user.
// TODO: implement against POST /api/v1/users/{provider_id}/follow  (verify path).
func (c *Client) Follow(ctx context.Context, accountID, providerID string) error {
	if c.dryRun {
		return nil
	}
	return ErrNotImplemented
}

// LikePost likes a post.
func (c *Client) LikePost(ctx context.Context, accountID, postURN string) error {
	if c.dryRun {
		return nil
	}
	return ErrNotImplemented
}

// CommentPost comments on a post.
func (c *Client) CommentPost(ctx context.Context, accountID, postURN, text string) error {
	if c.dryRun {
		return nil
	}
	return ErrNotImplemented
}

// WithdrawInvite withdraws a previously-sent invitation.
func (c *Client) WithdrawInvite(ctx context.Context, accountID, invitationID string) error {
	if c.dryRun {
		return nil
	}
	return ErrNotImplemented
}

// SendVoiceNote sends a voice-note attachment.
func (c *Client) SendVoiceNote(ctx context.Context, chatID, audioURL string) error {
	if c.dryRun {
		return nil
	}
	return ErrNotImplemented
}

// SendInMail sends a paid InMail with subject.
func (c *Client) SendInMail(ctx context.Context, accountID, providerID, subject, text string) error {
	if c.dryRun {
		return nil
	}
	return ErrNotImplemented
}
