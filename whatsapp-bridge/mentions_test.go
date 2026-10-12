package main

import (
	"reflect"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestParseMentionJID(t *testing.T) {
	cases := map[string]string{
		"31651053590":                "31651053590@s.whatsapp.net",
		"+31 6 5105 3590":            "31651053590@s.whatsapp.net",
		"31651053590@s.whatsapp.net": "31651053590@s.whatsapp.net",
		"132890717384852@lid":        "132890717384852@lid",
	}
	for in, want := range cases {
		got, err := parseMentionJID(in)
		if err != nil || got.String() != want {
			t.Errorf("parseMentionJID(%q) = %v, %v; want %s", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "Alex"} {
		if _, err := parseMentionJID(bad); err == nil {
			t.Errorf("parseMentionJID(%q) should fail", bad)
		}
	}
}

func TestApplyMentionsRewritesPhoneToLIDInGroup(t *testing.T) {
	lid := types.JID{User: "132890717384852", Server: types.HiddenUserServer}
	pn := types.JID{User: "31651053590", Server: types.DefaultUserServer}
	participants := []types.GroupParticipant{{JID: lid, LID: lid, PhoneNumber: pn}}

	text, jids, err := applyMentions("@31651053590 please check #664", []string{"31651053590"}, participants)
	if err != nil {
		t.Fatal(err)
	}
	if text != "@132890717384852 please check #664" {
		t.Errorf("text = %q", text)
	}
	if !reflect.DeepEqual(jids, []string{"132890717384852@lid"}) {
		t.Errorf("jids = %v", jids)
	}
}

func TestApplyMentionsLIDGivenDirectly(t *testing.T) {
	lid := types.JID{User: "132890717384852", Server: types.HiddenUserServer}
	participants := []types.GroupParticipant{{JID: lid, LID: lid}}
	text, jids, err := applyMentions("hi @132890717384852, see issue", []string{"132890717384852@lid"}, participants)
	if err != nil || text != "hi @132890717384852, see issue" || len(jids) != 1 {
		t.Errorf("got %q %v %v", text, jids, err)
	}
}

func TestApplyMentionsWithoutGroupKeepsGivenJID(t *testing.T) {
	text, jids, err := applyMentions("@31651053590", []string{"31651053590"}, nil)
	if err != nil || text != "@31651053590" || !reflect.DeepEqual(jids, []string{"31651053590@s.whatsapp.net"}) {
		t.Errorf("got %q %v %v", text, jids, err)
	}
}

func TestApplyMentionsRequiresTokenInText(t *testing.T) {
	if _, _, err := applyMentions("no token here", []string{"31651053590"}, nil); err == nil {
		t.Error("expected an error when the @token is missing")
	}
}

func TestApplyMentionsDoesNotMatchLongerNumber(t *testing.T) {
	// "@316510535901" is a different number than "@31651053590"
	if _, _, err := applyMentions("@316510535901", []string{"31651053590"}, nil); err == nil {
		t.Error("a longer number must not count as the mentioned one")
	}
}

func TestApplyMentionsTwoPeopleAndDuplicates(t *testing.T) {
	a := types.JID{User: "111", Server: types.HiddenUserServer}
	b := types.JID{User: "222", Server: types.HiddenUserServer}
	participants := []types.GroupParticipant{
		{JID: a, LID: a, PhoneNumber: types.JID{User: "31611111111", Server: types.DefaultUserServer}},
		{JID: b, LID: b, PhoneNumber: types.JID{User: "31622222222", Server: types.DefaultUserServer}},
	}
	text, jids, err := applyMentions("@31611111111 and @31622222222, @31611111111 again", []string{"31611111111", "31622222222", "31611111111"}, participants)
	if err != nil {
		t.Fatal(err)
	}
	if text != "@111 and @222, @111 again" {
		t.Errorf("text = %q", text)
	}
	if !reflect.DeepEqual(jids, []string{"111@lid", "222@lid"}) {
		t.Errorf("jids = %v", jids)
	}
}
