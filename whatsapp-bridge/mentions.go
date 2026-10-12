package main

import (
	"fmt"
	"regexp"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// Real WhatsApp mentions.
//
// A plain-text message ("Conversation") that contains "@Alex" or "@132890717384852" is just text:
// WhatsApp only turns it into a mention (notification + the contact's name) when the message is an
// ExtendedTextMessage whose ContextInfo.MentionedJID lists the person AND the text contains
// "@<user part of that JID>" at the place where the name should appear.
//
// Callers pass `mentions` as phone numbers or JIDs and write "@<the same number>" in the text. In
// groups WhatsApp addresses people by their LID (the long internal number), so the given number is
// matched against the group's participants (phone number or LID) and the text token is rewritten
// to the participant's primary JID user part, so either form can be written in the text.

// parseMentionJID turns "31651053590", "+31 6 5105 3590", "31651053590@s.whatsapp.net" or
// "132890717384852@lid" into a JID.
func parseMentionJID(m string) (types.JID, error) {
	m = strings.TrimSpace(m)
	if m == "" {
		return types.JID{}, fmt.Errorf("empty mention")
	}
	if strings.Contains(m, "@") {
		return types.ParseJID(m)
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, m)
	if digits == "" {
		return types.JID{}, fmt.Errorf("mention %q has no digits", m)
	}
	return types.JID{User: digits, Server: types.DefaultUserServer}, nil
}

// resolveMention finds the JID WhatsApp wants for this person in this group. Without a match (or
// outside a group) the given JID is used as it is.
func resolveMention(given types.JID, participants []types.GroupParticipant) types.JID {
	for _, p := range participants {
		if p.JID.User == given.User || p.PhoneNumber.User == given.User || p.LID.User == given.User {
			return p.JID
		}
	}
	return given
}

// applyMentions rewrites the "@<number>" tokens in text and returns the new text plus the JID
// strings for ContextInfo.MentionedJID. It fails when a mention has no "@<number>" token in the text,
// because WhatsApp would then notify the person without showing where they were mentioned.
func applyMentions(text string, mentions []string, participants []types.GroupParticipant) (string, []string, error) {
	jids := make([]string, 0, len(mentions))
	seen := map[string]bool{}
	for _, m := range mentions {
		given, err := parseMentionJID(m)
		if err != nil {
			return "", nil, err
		}
		resolved := resolveMention(given, participants)
		if seen[resolved.String()] {
			// Same person listed twice (or as phone and as LID): the first pass already rewrote every token.
			continue
		}

		token := regexp.MustCompile("@" + regexp.QuoteMeta(given.User) + `(\D|$)`)
		if !token.MatchString(text) {
			return "", nil, fmt.Errorf("the text must contain @%s where %s is mentioned", given.User, m)
		}
		text = token.ReplaceAllString(text, "@"+resolved.User+"${1}")

		if !seen[resolved.String()] {
			seen[resolved.String()] = true
			jids = append(jids, resolved.String())
		}
	}
	return text, jids, nil
}
