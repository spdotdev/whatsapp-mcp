import os
import sqlite3
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from whatsapp import list_calls  # noqa: E402


def _seed(db_path):
    conn = sqlite3.connect(db_path)
    conn.execute(
        """CREATE TABLE calls (
            id TEXT PRIMARY KEY, chat_jid TEXT, caller TEXT, call_type TEXT,
            direction TEXT, status TEXT, start_time TIMESTAMP, accept_time TIMESTAMP,
            end_time TIMESTAMP, duration_seconds INTEGER
        )"""
    )
    conn.executemany(
        "INSERT INTO calls VALUES (?,?,?,?,?,?,?,?,?,?)",
        [
            ("C1", "111@s.whatsapp.net", "111", "voice", "incoming", "answered",
             "2026-06-26 09:00:00", "2026-06-26 09:00:05", "2026-06-26 09:02:05", 120),
            ("C2", "111@s.whatsapp.net", "111", "voice", "incoming", "missed",
             "2026-06-26 08:00:00", None, "2026-06-26 08:00:30", 0),
            ("C3", "999@s.whatsapp.net", "999", "video", "outgoing", "answered",
             "2026-06-26 07:00:00", "2026-06-26 07:00:02", "2026-06-26 07:10:00", 598),
        ],
    )
    conn.commit()
    conn.close()


def test_list_calls_filters_by_chat(tmp_path):
    db = str(tmp_path / "messages.db")
    _seed(db)
    calls = list_calls(chat_jid="111@s.whatsapp.net", db_path=db)
    assert [c.id for c in calls] == ["C1", "C2"]  # most recent first
    assert calls[0].status == "answered"
    assert calls[0].duration_seconds == 120


def test_list_calls_all_and_limit(tmp_path):
    db = str(tmp_path / "messages.db")
    _seed(db)
    assert len(list_calls(db_path=db)) == 3
    assert len(list_calls(limit=1, db_path=db)) == 1


def test_list_calls_time_window(tmp_path):
    db = str(tmp_path / "messages.db")
    _seed(db)
    calls = list_calls(after="2026-06-26 08:30:00", db_path=db)
    assert [c.id for c in calls] == ["C1"]
