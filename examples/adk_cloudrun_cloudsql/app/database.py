"""Database Session Service and State Storage for Cloud SQL & Local Engines.

This module provides persistence for ADK agents, managing:
1. Sessions (active conversation instances)
2. Events (chat history, tool calls, and model outputs)
3. Session State (scratchpad memory tied to a specific session)
4. User State (cross-session memory tied to a user_id, via 'user:' keys)

Supports both Google Cloud SQL (PostgreSQL with psycopg2) and SQLite (local dev/test).
"""
import json
import os
import sqlite3
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

try:
    import psycopg2
    import psycopg2.extras
    PSYCOPG2_AVAILABLE = True
except ImportError:
    PSYCOPG2_AVAILABLE = False


class DatabaseSessionManager:
    """Manages agent session history and memory state in SQL storage."""

    def __init__(self, db_url: Optional[str] = None):
        self.cloud_sql_instance = os.getenv("CLOUD_SQL_CONNECTION_NAME")
        self.db_user = os.getenv("DB_USER", "adk_agent_user")
        self.db_pass = os.getenv("DB_PASSWORD", "")
        self.db_name = os.getenv("DB_NAME", "adk_agent_db")

        self.db_url = db_url or os.getenv("DATABASE_URL", "")
        self._is_postgres = bool(self.cloud_sql_instance or (self.db_url and "postgres" in self.db_url)) and PSYCOPG2_AVAILABLE
        self._sqlite_path = (self.db_url.replace("sqlite:///", "").replace("sqlite+aiosqlite:///", "")
                             if self.db_url else "/tmp/adk_sessions.db")
        self._init_db()

    def _get_connection(self):
        """Returns a database connection (PostgreSQL or SQLite)."""
        if self._is_postgres:
            if self.cloud_sql_instance:
                # Cloud Run mounts Cloud SQL unix socket at /cloudsql/INSTANCE_CONNECTION_NAME
                return psycopg2.connect(
                    host=f"/cloudsql/{self.cloud_sql_instance}",
                    user=self.db_user,
                    password=self.db_pass,
                    dbname=self.db_name,
                    cursor_factory=psycopg2.extras.RealDictCursor,
                )
            else:
                return psycopg2.connect(self.db_url, cursor_factory=psycopg2.extras.RealDictCursor)
        else:
            conn = sqlite3.connect(self._sqlite_path)
            conn.row_factory = sqlite3.Row
            return conn

    def _init_db(self):
        """Creates tables if they do not exist."""
        try:
            with self._get_connection() as conn:
                cursor = conn.cursor()
                if self._is_postgres:
                    cursor.execute("""
                    CREATE TABLE IF NOT EXISTS sessions (
                        session_id TEXT PRIMARY KEY,
                        user_id TEXT NOT NULL,
                        created_at TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        title TEXT
                    );
                    CREATE TABLE IF NOT EXISTS events (
                        id SERIAL PRIMARY KEY,
                        session_id TEXT NOT NULL REFERENCES sessions(session_id) ON DELETE CASCADE,
                        role TEXT NOT NULL,
                        author TEXT,
                        content TEXT NOT NULL,
                        tool_calls TEXT,
                        created_at TEXT NOT NULL
                    );
                    CREATE TABLE IF NOT EXISTS session_states (
                        session_id TEXT NOT NULL,
                        key TEXT NOT NULL,
                        value_json TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        PRIMARY KEY (session_id, key)
                    );
                    CREATE TABLE IF NOT EXISTS user_states (
                        user_id TEXT NOT NULL,
                        key TEXT NOT NULL,
                        value_json TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        PRIMARY KEY (user_id, key)
                    );
                    """)
                else:
                    db_dir = os.path.dirname(self._sqlite_path)
                    if db_dir and not os.path.exists(db_dir):
                        os.makedirs(db_dir, exist_ok=True)
                    cursor.execute("""
                    CREATE TABLE IF NOT EXISTS sessions (
                        session_id TEXT PRIMARY KEY,
                        user_id TEXT NOT NULL,
                        created_at TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        title TEXT
                    );
                    """)
                    cursor.execute("""
                    CREATE TABLE IF NOT EXISTS events (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        session_id TEXT NOT NULL,
                        role TEXT NOT NULL,
                        author TEXT,
                        content TEXT NOT NULL,
                        tool_calls TEXT,
                        created_at TEXT NOT NULL,
                        FOREIGN KEY (session_id) REFERENCES sessions(session_id)
                    );
                    """)
                    cursor.execute("""
                    CREATE TABLE IF NOT EXISTS session_states (
                        session_id TEXT NOT NULL,
                        key TEXT NOT NULL,
                        value_json TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        PRIMARY KEY (session_id, key)
                    );
                    """)
                    cursor.execute("""
                    CREATE TABLE IF NOT EXISTS user_states (
                        user_id TEXT NOT NULL,
                        key TEXT NOT NULL,
                        value_json TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        PRIMARY KEY (user_id, key)
                    );
                    """)
                conn.commit()
        except Exception as e:
            # If PostgreSQL socket is not yet ready, fall back gracefully to local SQLite
            print(f"[DatabaseSessionManager] Initialization warning (falling back to SQLite): {e}")
            self._is_postgres = False
            self._init_db()

    def get_or_create_session(self, session_id: str, user_id: str = "default_user", title: str = "New Chat") -> Dict[str, Any]:
        """Retrieves or creates a session record."""
        now = datetime.now(timezone.utc).isoformat()
        ph = "%s" if self._is_postgres else "?"
        with self._get_connection() as conn:
            cursor = conn.cursor()
            cursor.execute(f"SELECT * FROM sessions WHERE session_id = {ph}", (session_id,))
            row = cursor.fetchone()
            if row:
                return dict(row)
            
            cursor.execute(
                f"INSERT INTO sessions (session_id, user_id, created_at, updated_at, title) VALUES ({ph}, {ph}, {ph}, {ph}, {ph})",
                (session_id, user_id, now, now, title),
            )
            conn.commit()
            return {
                "session_id": session_id,
                "user_id": user_id,
                "created_at": now,
                "updated_at": now,
                "title": title,
            }

    def list_sessions(self, user_id: Optional[str] = None) -> List[Dict[str, Any]]:
        """Lists sessions, optionally filtered by user_id."""
        ph = "%s" if self._is_postgres else "?"
        with self._get_connection() as conn:
            cursor = conn.cursor()
            if user_id:
                cursor.execute(f"SELECT * FROM sessions WHERE user_id = {ph} ORDER BY updated_at DESC", (user_id,))
            else:
                cursor.execute("SELECT * FROM sessions ORDER BY updated_at DESC")
            return [dict(row) for row in cursor.fetchall()]

    def append_event(self, session_id: str, role: str, content: str, author: Optional[str] = None, tool_calls: Optional[List[Dict[str, Any]]] = None) -> int:
        """Appends a conversation event (message, tool call, or response)."""
        now = datetime.now(timezone.utc).isoformat()
        tools_str = json.dumps(tool_calls) if tool_calls else None
        ph = "%s" if self._is_postgres else "?"
        with self._get_connection() as conn:
            cursor = conn.cursor()
            if self._is_postgres:
                cursor.execute(
                    f"INSERT INTO events (session_id, role, author, content, tool_calls, created_at) VALUES ({ph}, {ph}, {ph}, {ph}, {ph}, {ph}) RETURNING id",
                    (session_id, role, author or role, content, tools_str, now),
                )
                last_id = cursor.fetchone()["id"]
            else:
                cursor.execute(
                    f"INSERT INTO events (session_id, role, author, content, tool_calls, created_at) VALUES ({ph}, {ph}, {ph}, {ph}, {ph}, {ph})",
                    (session_id, role, author or role, content, tools_str, now),
                )
                last_id = cursor.lastrowid
            cursor.execute(f"UPDATE sessions SET updated_at = {ph} WHERE session_id = {ph}", (now, session_id))
            conn.commit()
            return last_id

    def get_events(self, session_id: str) -> List[Dict[str, Any]]:
        """Retrieves all events for a given session."""
        ph = "%s" if self._is_postgres else "?"
        with self._get_connection() as conn:
            cursor = conn.cursor()
            cursor.execute(f"SELECT * FROM events WHERE session_id = {ph} ORDER BY id ASC", (session_id,))
            events = []
            for row in cursor.fetchall():
                e = dict(row)
                if e.get("tool_calls"):
                    e["tool_calls"] = json.loads(e["tool_calls"])
                events.append(e)
            return events

    def load_combined_state(self, session_id: str, user_id: str) -> Dict[str, Any]:
        """Loads both user-scoped memory (prefixed with 'user:') and session-scoped state."""
        state: Dict[str, Any] = {}
        ph = "%s" if self._is_postgres else "?"
        with self._get_connection() as conn:
            cursor = conn.cursor()
            # 1. Load user states (long-term memory across sessions)
            cursor.execute(f"SELECT key, value_json FROM user_states WHERE user_id = {ph}", (user_id,))
            for row in cursor.fetchall():
                key = row["key"]
                val = json.loads(row["value_json"])
                state[f"user:{key}"] = val

            # 2. Load session-specific state
            cursor.execute(f"SELECT key, value_json FROM session_states WHERE session_id = {ph}", (session_id,))
            for row in cursor.fetchall():
                key = row["key"]
                val = json.loads(row["value_json"])
                state[key] = val

        return state

    def save_combined_state(self, session_id: str, user_id: str, state_dict: Dict[str, Any]):
        """Persists state, routing 'user:' prefixed keys to user_states and others to session_states."""
        now = datetime.now(timezone.utc).isoformat()
        ph = "%s" if self._is_postgres else "?"
        with self._get_connection() as conn:
            cursor = conn.cursor()
            for k, v in state_dict.items():
                val_json = json.dumps(v)
                if k.startswith("user:"):
                    clean_key = k[len("user:"):]
                    cursor.execute(f"""
                        INSERT INTO user_states (user_id, key, value_json, updated_at)
                        VALUES ({ph}, {ph}, {ph}, {ph})
                        ON CONFLICT(user_id, key) DO UPDATE SET
                            value_json = excluded.value_json,
                            updated_at = excluded.updated_at
                    """, (user_id, clean_key, val_json, now))
                else:
                    cursor.execute(f"""
                        INSERT INTO session_states (session_id, key, value_json, updated_at)
                        VALUES ({ph}, {ph}, {ph}, {ph})
                        ON CONFLICT(session_id, key) DO UPDATE SET
                            value_json = excluded.value_json,
                            updated_at = excluded.updated_at
                    """, (session_id, k, val_json, now))
            conn.commit()


# Singleton instance for application usage
db_manager = DatabaseSessionManager()
