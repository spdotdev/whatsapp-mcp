package main

import (
	"database/sql"
	"fmt"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// A WhatsApp reply is a message whose ContextInfo points at the quoted message: its id (StanzaID),
// its author (Participant) and a copy of its text (QuotedMessage). Without all three the phone shows
// a plain message, not a linked reply.

// resolveReplySender turns the sender stored for the quoted message (a bare user number, phone or LID)
// into the JID WhatsApp wants as Participant. In a group the member is looked up first, the same way
// mentions are, because groups address people by LID. Without a match the fallback is used when given,
// else the number is taken as a phone JID.
func resolveReplySender(sender string, participants []types.GroupParticipant, fallback types.JID) types.JID {
	for _, p := range participants {
		if p.JID.User == sender || p.PhoneNumber.User == sender || p.LID.User == sender {
			return p.JID
		}
	}
	if !fallback.IsEmpty() {
		return fallback
	}
	return types.JID{User: sender, Server: types.DefaultUserServer}
}

// buildReplyContext returns the ContextInfo that makes a message a reply to quotedID.
func buildReplyContext(quotedID, quotedText string, quotedSender types.JID) *waProto.ContextInfo {
	return &waProto.ContextInfo{
		StanzaID:      proto.String(quotedID),
		Participant:   proto.String(quotedSender.String()),
		QuotedMessage: &waProto.Message{Conversation: proto.String(quotedText)},
	}
}

// applyReply fills the reply fields of ctx for the message quotedID in chatJID, looked up in the local
// message store. own is this account's JID, used when the quoted message was sent by us.
func applyReply(ctx *waProto.ContextInfo, store *MessageStore, quotedID string, chatJID, own types.JID, participants []types.GroupParticipant) error {
	var sender, content string
	var isFromMe bool
	err := store.db.QueryRow(`SELECT sender, content, is_from_me FROM messages WHERE id = ? AND chat_jid = ?`,
		quotedID, chatJID.String()).Scan(&sender, &content, &isFromMe)
	if err == sql.ErrNoRows {
		return fmt.Errorf("message %s not found in chat %s, cannot reply to it", quotedID, chatJID)
	}
	if err != nil {
		return fmt.Errorf("reading the message to reply to: %w", err)
	}

	var author types.JID
	if isFromMe {
		author = own
	} else {
		fallback := types.JID{}
		if chatJID.Server != types.GroupServer {
			fallback = chatJID
		}
		author = resolveReplySender(sender, participants, fallback)
	}
	r := buildReplyContext(quotedID, content, author)
	ctx.StanzaID, ctx.Participant, ctx.QuotedMessage = r.StanzaID, r.Participant, r.QuotedMessage
	return nil
}
