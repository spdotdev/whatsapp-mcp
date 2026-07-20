import sqlite3
from datetime import datetime
from dataclasses import dataclass
from typing import Optional, List, Tuple
import os
import os.path
import requests
import json
import audio

# Only used as a fallback DB path for test-only direct-sqlite access (see
# list_calls' db_path parameter). Normal reads go over the bridge's HTTP API.
MESSAGES_DB_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'whatsapp-bridge', 'store', 'messages.db')
WHATSAPP_API_BASE_URL = os.environ.get("WHATSAPP_API_BASE_URL", "http://localhost:8080/api")
_WHATSAPP_API_USERNAME = os.environ.get("WHATSAPP_API_USERNAME")
_WHATSAPP_API_PASSWORD = os.environ.get("WHATSAPP_API_PASSWORD")
WHATSAPP_API_AUTH = (
    (_WHATSAPP_API_USERNAME, _WHATSAPP_API_PASSWORD)
    if _WHATSAPP_API_USERNAME and _WHATSAPP_API_PASSWORD
    else None
)

@dataclass
class Message:
    timestamp: datetime
    sender: str
    content: str
    is_from_me: bool
    chat_jid: str
    id: str
    chat_name: Optional[str] = None
    media_type: Optional[str] = None

@dataclass
class Chat:
    jid: str
    name: Optional[str]
    last_message_time: Optional[datetime]
    last_message: Optional[str] = None
    last_sender: Optional[str] = None
    last_is_from_me: Optional[bool] = None

    @property
    def is_group(self) -> bool:
        """Determine if chat is a group based on JID pattern."""
        return self.jid.endswith("@g.us")

@dataclass
class Contact:
    phone_number: str
    name: Optional[str]
    jid: str

@dataclass
class Call:
    id: str
    chat_jid: str
    caller: str
    call_type: str
    direction: str
    status: str
    start_time: str
    end_time: Optional[str] = None
    duration_seconds: int = 0

@dataclass
class MessageContext:
    message: Message
    before: List[Message]
    after: List[Message]

def _normalize_iso_datetime(value: str) -> str:
    """Parse an ISO-8601 datetime string and re-render it the same way the
    stdlib sqlite3 adapter used to (space-separated, no timezone), so
    filters sent to the bridge match the format stored in the DB. Raises
    ValueError on invalid input, matching the previous behavior."""
    dt = datetime.fromisoformat(value)
    return dt.strftime("%Y-%m-%d %H:%M:%S")


def _message_from_json(data: dict) -> Message:
    return Message(
        timestamp=datetime.fromisoformat(data["timestamp"]),
        sender=data["sender"],
        chat_name=data.get("chat_name"),
        content=data["content"],
        is_from_me=data["is_from_me"],
        chat_jid=data["chat_jid"],
        id=data["id"],
        media_type=data.get("media_type"),
    )


def _chat_from_json(data: dict) -> Chat:
    return Chat(
        jid=data["jid"],
        name=data.get("name"),
        last_message_time=datetime.fromisoformat(data["last_message_time"]) if data.get("last_message_time") else None,
        last_message=data.get("last_message"),
        last_sender=data.get("last_sender"),
        last_is_from_me=data.get("last_is_from_me"),
    )


def get_sender_name(sender_jid: str) -> str:
    try:
        response = requests.get(f"{WHATSAPP_API_BASE_URL}/contacts/resolve", params={"jid": sender_jid}, auth=WHATSAPP_API_AUTH)
        if response.status_code == 200:
            return response.json().get("name") or sender_jid
        return sender_jid
    except requests.RequestException as e:
        print(f"Database error while getting sender name: {e}")
        return sender_jid

def format_message(message: Message, show_chat_info: bool = True) -> None:
    """Print a single message with consistent formatting."""
    output = ""
    
    if show_chat_info and message.chat_name:
        output += f"[{message.timestamp:%Y-%m-%d %H:%M:%S}] Chat: {message.chat_name} "
    else:
        output += f"[{message.timestamp:%Y-%m-%d %H:%M:%S}] "
        
    content_prefix = ""
    if hasattr(message, 'media_type') and message.media_type:
        content_prefix = f"[{message.media_type} - Message ID: {message.id} - Chat JID: {message.chat_jid}] "
    
    try:
        sender_name = get_sender_name(message.sender) if not message.is_from_me else "Me"
        output += f"From: {sender_name}: {content_prefix}{message.content}\n"
    except Exception as e:
        print(f"Error formatting message: {e}")
    return output

def format_messages_list(messages: List[Message], show_chat_info: bool = True) -> None:
    output = ""
    if not messages:
        output += "No messages to display."
        return output
    
    for message in messages:
        output += format_message(message, show_chat_info)
    return output

def list_messages(
    after: Optional[str] = None,
    before: Optional[str] = None,
    sender_phone_number: Optional[str] = None,
    chat_jid: Optional[str] = None,
    query: Optional[str] = None,
    limit: int = 20,
    page: int = 0,
    include_context: bool = True,
    context_before: int = 1,
    context_after: int = 1
) -> List[Message]:
    """Get messages matching the specified criteria with optional context."""
    try:
        params = {"limit": limit, "page": page}

        if after:
            try:
                params["after"] = _normalize_iso_datetime(after)
            except ValueError:
                raise ValueError(f"Invalid date format for 'after': {after}. Please use ISO-8601 format.")

        if before:
            try:
                params["before"] = _normalize_iso_datetime(before)
            except ValueError:
                raise ValueError(f"Invalid date format for 'before': {before}. Please use ISO-8601 format.")

        if sender_phone_number:
            params["sender"] = sender_phone_number

        if chat_jid:
            params["chat_jid"] = chat_jid

        if query:
            params["query"] = query

        response = requests.get(f"{WHATSAPP_API_BASE_URL}/messages", params=params, auth=WHATSAPP_API_AUTH)
        response.raise_for_status()
        result = [_message_from_json(msg) for msg in response.json()]

        if include_context and result:
            # Add context for each message
            messages_with_context = []
            for msg in result:
                context = get_message_context(msg.id, context_before, context_after)
                messages_with_context.extend(context.before)
                messages_with_context.append(context.message)
                messages_with_context.extend(context.after)

            return format_messages_list(messages_with_context, show_chat_info=True)

        # Format and display messages without context
        return format_messages_list(result, show_chat_info=True)

    except requests.RequestException as e:
        print(f"Database error: {e}")
        return []


def get_message_context(
    message_id: str,
    before: int = 5,
    after: int = 5
) -> MessageContext:
    """Get context around a specific message."""
    try:
        response = requests.get(
            f"{WHATSAPP_API_BASE_URL}/messages/context",
            params={"message_id": message_id, "before": before, "after": after},
            auth=WHATSAPP_API_AUTH
        )
        if response.status_code == 404:
            raise ValueError(f"Message with ID {message_id} not found")
        response.raise_for_status()
        data = response.json()

        return MessageContext(
            message=_message_from_json(data["message"]),
            before=[_message_from_json(m) for m in data["before"]],
            after=[_message_from_json(m) for m in data["after"]],
        )

    except requests.RequestException as e:
        print(f"Database error: {e}")
        raise


def list_chats(
    query: Optional[str] = None,
    limit: int = 20,
    page: int = 0,
    include_last_message: bool = True,
    sort_by: str = "last_active"
) -> List[Chat]:
    """Get chats matching the specified criteria."""
    try:
        params = {
            "limit": limit,
            "page": page,
            "include_last_message": str(include_last_message).lower(),
            "sort_by": sort_by,
        }
        if query:
            params["query"] = query

        response = requests.get(f"{WHATSAPP_API_BASE_URL}/chats", params=params, auth=WHATSAPP_API_AUTH)
        response.raise_for_status()
        return [_chat_from_json(c) for c in response.json()]

    except requests.RequestException as e:
        print(f"Database error: {e}")
        return []


def search_contacts(query: str) -> List[Contact]:
    """Search contacts by name or phone number."""
    try:
        response = requests.get(f"{WHATSAPP_API_BASE_URL}/contacts/search", params={"query": query}, auth=WHATSAPP_API_AUTH)
        response.raise_for_status()

        result = []
        for contact_data in response.json():
            jid = contact_data["jid"]
            contact = Contact(
                phone_number=jid.split('@')[0],
                name=contact_data.get("name"),
                jid=jid
            )
            result.append(contact)

        return result

    except requests.RequestException as e:
        print(f"Database error: {e}")
        return []


def get_contact_chats(jid: str, limit: int = 20, page: int = 0) -> List[Chat]:
    """Get all chats involving the contact.
    
    Args:
        jid: The contact's JID to search for
        limit: Maximum number of chats to return (default 20)
        page: Page number for pagination (default 0)
    """
    try:
        response = requests.get(
            f"{WHATSAPP_API_BASE_URL}/contacts/chats",
            params={"jid": jid, "limit": limit, "page": page},
            auth=WHATSAPP_API_AUTH
        )
        response.raise_for_status()
        return [_chat_from_json(c) for c in response.json()]

    except requests.RequestException as e:
        print(f"Database error: {e}")
        return []


def get_last_interaction(jid: str) -> str:
    """Get most recent message involving the contact."""
    try:
        response = requests.get(f"{WHATSAPP_API_BASE_URL}/last-interaction", params={"jid": jid}, auth=WHATSAPP_API_AUTH)
        if response.status_code == 404:
            return None
        response.raise_for_status()

        message = _message_from_json(response.json())
        return format_message(message)

    except requests.RequestException as e:
        print(f"Database error: {e}")
        return None


def list_calls(
    chat_jid: Optional[str] = None,
    after: Optional[str] = None,
    before: Optional[str] = None,
    limit: int = 20,
    db_path: Optional[str] = None,
) -> List[Call]:
    """List WhatsApp voice/video calls, most recent first.

    Args:
        chat_jid: optional chat JID filter
        after / before: optional ISO-8601 datetime bounds on start_time
        limit: max rows
        db_path: override DB path — direct-sqlite escape hatch used only by
            the test suite, which seeds an isolated temp DB. Normal runtime
            callers must leave this unset, which routes the read through the
            bridge's HTTP API (works even when the bridge runs remotely).
    """
    if db_path is not None:
        conn = sqlite3.connect(db_path)
        try:
            cursor = conn.cursor()
            clauses, params = [], []
            if chat_jid:
                clauses.append("chat_jid = ?")
                params.append(chat_jid)
            if after:
                try:
                    after = datetime.fromisoformat(after)
                except ValueError:
                    raise ValueError(f"Invalid date format for 'after': {after}. Please use ISO-8601 format.")
                clauses.append("start_time >= ?")
                params.append(after)
            if before:
                try:
                    before = datetime.fromisoformat(before)
                except ValueError:
                    raise ValueError(f"Invalid date format for 'before': {before}. Please use ISO-8601 format.")
                clauses.append("start_time <= ?")
                params.append(before)
            where = ("WHERE " + " AND ".join(clauses)) if clauses else ""
            params.append(limit)
            cursor.execute(
                f"""SELECT id, chat_jid, caller, call_type, direction, status,
                           start_time, end_time, duration_seconds
                    FROM calls {where} ORDER BY start_time DESC LIMIT ?""",
                params,
            )
            calls = []
            for row in cursor.fetchall():
                calls.append(
                    Call(
                        id=row[0], chat_jid=row[1], caller=row[2], call_type=row[3],
                        direction=row[4], status=row[5],
                        start_time=row[6], end_time=row[7],
                        duration_seconds=row[8] or 0,
                    )
                )
            return calls
        finally:
            conn.close()

    params = {"limit": limit}
    if chat_jid:
        params["chat_jid"] = chat_jid
    if after:
        try:
            params["after"] = _normalize_iso_datetime(after)
        except ValueError:
            raise ValueError(f"Invalid date format for 'after': {after}. Please use ISO-8601 format.")
    if before:
        try:
            params["before"] = _normalize_iso_datetime(before)
        except ValueError:
            raise ValueError(f"Invalid date format for 'before': {before}. Please use ISO-8601 format.")

    response = requests.get(f"{WHATSAPP_API_BASE_URL}/calls", params=params, auth=WHATSAPP_API_AUTH)
    response.raise_for_status()
    calls = []
    for row in response.json():
        calls.append(
            Call(
                id=row["id"], chat_jid=row["chat_jid"], caller=row["caller"], call_type=row["call_type"],
                direction=row["direction"], status=row["status"],
                start_time=row["start_time"], end_time=row.get("end_time"),
                duration_seconds=row.get("duration_seconds") or 0,
            )
        )
    return calls


def get_chat(chat_jid: str, include_last_message: bool = True) -> Optional[Chat]:
    """Get chat metadata by JID."""
    try:
        response = requests.get(
            f"{WHATSAPP_API_BASE_URL}/chat",
            params={"jid": chat_jid, "include_last_message": str(include_last_message).lower()},
            auth=WHATSAPP_API_AUTH
        )
        if response.status_code == 404:
            return None
        response.raise_for_status()
        return _chat_from_json(response.json())

    except requests.RequestException as e:
        print(f"Database error: {e}")
        return None


def get_direct_chat_by_contact(sender_phone_number: str) -> Optional[Chat]:
    """Get chat metadata by sender phone number."""
    try:
        response = requests.get(f"{WHATSAPP_API_BASE_URL}/chat/direct", params={"phone": sender_phone_number}, auth=WHATSAPP_API_AUTH)
        if response.status_code == 404:
            return None
        response.raise_for_status()
        return _chat_from_json(response.json())

    except requests.RequestException as e:
        print(f"Database error: {e}")
        return None

def send_message(recipient: str, message: str) -> Tuple[bool, str]:
    try:
        # Validate input
        if not recipient:
            return False, "Recipient must be provided"
        
        url = f"{WHATSAPP_API_BASE_URL}/send"
        payload = {
            "recipient": recipient,
            "message": message,
        }
        
        response = requests.post(url, json=payload, auth=WHATSAPP_API_AUTH)
        
        # Check if the request was successful
        if response.status_code == 200:
            result = response.json()
            return result.get("success", False), result.get("message", "Unknown response")
        else:
            return False, f"Error: HTTP {response.status_code} - {response.text}"
            
    except requests.RequestException as e:
        return False, f"Request error: {str(e)}"
    except json.JSONDecodeError:
        return False, f"Error parsing response: {response.text}"
    except Exception as e:
        return False, f"Unexpected error: {str(e)}"

def send_file(recipient: str, media_path: str) -> Tuple[bool, str]:
    try:
        # Validate input
        if not recipient:
            return False, "Recipient must be provided"
        
        if not media_path:
            return False, "Media path must be provided"
        
        if not os.path.isfile(media_path):
            return False, f"Media file not found: {media_path}"
        
        url = f"{WHATSAPP_API_BASE_URL}/send"
        payload = {
            "recipient": recipient,
            "media_path": media_path
        }
        
        response = requests.post(url, json=payload, auth=WHATSAPP_API_AUTH)
        
        # Check if the request was successful
        if response.status_code == 200:
            result = response.json()
            return result.get("success", False), result.get("message", "Unknown response")
        else:
            return False, f"Error: HTTP {response.status_code} - {response.text}"
            
    except requests.RequestException as e:
        return False, f"Request error: {str(e)}"
    except json.JSONDecodeError:
        return False, f"Error parsing response: {response.text}"
    except Exception as e:
        return False, f"Unexpected error: {str(e)}"

def send_audio_message(recipient: str, media_path: str) -> Tuple[bool, str]:
    try:
        # Validate input
        if not recipient:
            return False, "Recipient must be provided"
        
        if not media_path:
            return False, "Media path must be provided"
        
        if not os.path.isfile(media_path):
            return False, f"Media file not found: {media_path}"

        if not media_path.endswith(".ogg"):
            try:
                media_path = audio.convert_to_opus_ogg_temp(media_path)
            except Exception as e:
                return False, f"Error converting file to opus ogg. You likely need to install ffmpeg: {str(e)}"
        
        url = f"{WHATSAPP_API_BASE_URL}/send"
        payload = {
            "recipient": recipient,
            "media_path": media_path
        }
        
        response = requests.post(url, json=payload, auth=WHATSAPP_API_AUTH)
        
        # Check if the request was successful
        if response.status_code == 200:
            result = response.json()
            return result.get("success", False), result.get("message", "Unknown response")
        else:
            return False, f"Error: HTTP {response.status_code} - {response.text}"
            
    except requests.RequestException as e:
        return False, f"Request error: {str(e)}"
    except json.JSONDecodeError:
        return False, f"Error parsing response: {response.text}"
    except Exception as e:
        return False, f"Unexpected error: {str(e)}"

def download_media(message_id: str, chat_jid: str) -> Optional[str]:
    """Download media from a message and return the local file path.
    
    Args:
        message_id: The ID of the message containing the media
        chat_jid: The JID of the chat containing the message
    
    Returns:
        The local file path if download was successful, None otherwise
    """
    try:
        url = f"{WHATSAPP_API_BASE_URL}/download"
        payload = {
            "message_id": message_id,
            "chat_jid": chat_jid
        }
        
        response = requests.post(url, json=payload, auth=WHATSAPP_API_AUTH)
        
        if response.status_code == 200:
            result = response.json()
            if result.get("success", False):
                path = result.get("path")
                print(f"Media downloaded successfully: {path}")
                return path
            else:
                print(f"Download failed: {result.get('message', 'Unknown error')}")
                return None
        else:
            print(f"Error: HTTP {response.status_code} - {response.text}")
            return None
            
    except requests.RequestException as e:
        print(f"Request error: {str(e)}")
        return None
    except json.JSONDecodeError:
        print(f"Error parsing response: {response.text}")
        return None
    except Exception as e:
        print(f"Unexpected error: {str(e)}")
        return None

def revoke_message(message_id: str, chat_jid: str) -> Tuple[bool, str]:
    """Revoke ("delete for everyone") a message we previously sent.

    Args:
        message_id: The ID of the message to revoke
        chat_jid: The JID of the chat the message is in

    Returns:
        A tuple of (success, status_message)
    """
    try:
        if not message_id:
            return False, "Message ID must be provided"
        if not chat_jid:
            return False, "Chat JID must be provided"

        url = f"{WHATSAPP_API_BASE_URL}/revoke"
        payload = {
            "message_id": message_id,
            "chat_jid": chat_jid,
        }

        response = requests.post(url, json=payload, auth=WHATSAPP_API_AUTH)

        if response.status_code == 200:
            result = response.json()
            return result.get("success", False), result.get("message", "Unknown response")
        else:
            return False, f"Error: HTTP {response.status_code} - {response.text}"

    except requests.RequestException as e:
        return False, f"Request error: {str(e)}"
    except json.JSONDecodeError:
        return False, f"Error parsing response: {response.text}"
    except Exception as e:
        return False, f"Unexpected error: {str(e)}"
