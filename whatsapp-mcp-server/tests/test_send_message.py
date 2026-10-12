import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import whatsapp  # noqa: E402


class _Response:
    status_code = 200
    text = "{}"

    def json(self):
        return {"success": True, "message": "ok"}


def _capture(monkeypatch):
    sent = {}

    def fake_post(url, json=None, auth=None):
        sent["url"] = url
        sent["payload"] = json
        return _Response()

    monkeypatch.setattr(whatsapp.requests, "post", fake_post)
    return sent


def test_send_message_without_mentions_keeps_the_old_payload(monkeypatch):
    sent = _capture(monkeypatch)
    ok, _ = whatsapp.send_message("123@g.us", "hello")
    assert ok
    assert sent["payload"] == {"recipient": "123@g.us", "message": "hello"}


def test_send_message_passes_mentions_to_the_bridge(monkeypatch):
    sent = _capture(monkeypatch)
    ok, _ = whatsapp.send_message("123@g.us", "@31651053590 hi", ["31651053590"])
    assert ok
    assert sent["payload"] == {
        "recipient": "123@g.us",
        "message": "@31651053590 hi",
        "mentions": ["31651053590"],
    }


def test_send_message_empty_mentions_list_is_not_sent(monkeypatch):
    sent = _capture(monkeypatch)
    whatsapp.send_message("123@g.us", "hello", [])
    assert "mentions" not in sent["payload"]


def test_send_message_passes_reply_to_the_bridge(monkeypatch):
    sent = _capture(monkeypatch)
    ok, _ = whatsapp.send_message("123@g.us", "answer", reply_to="3A918973CC7E976E622C")
    assert ok
    assert sent["payload"] == {
        "recipient": "123@g.us",
        "message": "answer",
        "reply_to": "3A918973CC7E976E622C",
    }


def test_send_message_without_reply_to_does_not_send_the_key(monkeypatch):
    sent = _capture(monkeypatch)
    whatsapp.send_message("123@g.us", "hello", reply_to=None)
    assert "reply_to" not in sent["payload"]
