package main

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestResolveReplySender(t *testing.T) {
	lid := types.JID{User: "120598487380186", Server: types.HiddenUserServer}
	pn := types.JID{User: "31651053590", Server: types.DefaultUserServer}
	participants := []types.GroupParticipant{
		{JID: lid, LID: lid},
		{JID: types.JID{User: "132890717384852", Server: types.HiddenUserServer}, PhoneNumber: pn},
	}

	if got := resolveReplySender("120598487380186", participants, types.JID{}); got.String() != lid.String() {
		t.Errorf("LID sender: got %s, want %s", got, lid)
	}
	if got := resolveReplySender("31651053590", participants, types.JID{}); got.User != "132890717384852" {
		t.Errorf("phone sender should map to the member's primary JID, got %s", got)
	}
	// Not a group member (or a direct chat): fall back to a plain phone JID.
	if got := resolveReplySender("999", nil, types.JID{}); got.String() != "999@s.whatsapp.net" {
		t.Errorf("fallback: got %s", got)
	}
}

func TestBuildReplyContext(t *testing.T) {
	sender := types.JID{User: "120598487380186", Server: types.HiddenUserServer}
	ctx := buildReplyContext("3A918973CC7E976E622C", "2 questions for tomorrow", sender)
	if ctx.GetStanzaID() != "3A918973CC7E976E622C" {
		t.Errorf("stanza id: %q", ctx.GetStanzaID())
	}
	if ctx.GetParticipant() != sender.String() {
		t.Errorf("participant: %q", ctx.GetParticipant())
	}
	if ctx.GetQuotedMessage().GetConversation() != "2 questions for tomorrow" {
		t.Errorf("quoted text: %q", ctx.GetQuotedMessage().GetConversation())
	}
}
