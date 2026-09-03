"""Integration tests for Cloud SQL and local database session memory persistence."""
import os
import sys
import shutil
import tempfile
import unittest

pkg_root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if pkg_root not in sys.path:
    sys.path.insert(0, pkg_root)

from app.database import DatabaseSessionManager


class TestDatabaseMemoryPersistence(unittest.TestCase):
    """Verifies state separation between session-scoped scratchpads and user-scoped memory."""

    def setUp(self):
        # Create an isolated temporary SQLite database for testing
        self.test_dir = tempfile.mkdtemp()
        self.db_path = os.path.join(self.test_dir, "test_sessions.db")
        self.db = DatabaseSessionManager(f"sqlite:///{self.db_path}")

    def tearDown(self):
        shutil.rmtree(self.test_dir)

    def test_session_lifecycle(self):
        """Verifies session creation, listing, and event appending."""
        session = self.db.get_or_create_session(session_id="sess_001", user_id="user_bob", title="Bob's Chat")
        self.assertEqual(session["session_id"], "sess_001")
        self.assertEqual(session["user_id"], "user_bob")

        # Append messages
        ev1 = self.db.append_event("sess_001", role="user", content="Hello agent!")
        ev2 = self.db.append_event("sess_001", role="model", content="Hello Bob!", author="cloudsql_memory_agent")
        
        events = self.db.get_events("sess_001")
        self.assertEqual(len(events), 2)
        self.assertEqual(events[0]["content"], "Hello agent!")
        self.assertEqual(events[1]["content"], "Hello Bob!")

    def test_state_separation_and_cross_session_persistence(self):
        """Crucial test:
        - Session 1: Saves user preference (user:coding_language = 'Go') and session goal ('Learn Cosm').
        - Session 2 (same user): User preference ('Go') MUST persist, but session goal MUST be reset.
        """
        user_id = "user_charlie"
        session1_id = "session_charlie_1"
        session2_id = "session_charlie_2"

        # 1. Setup Session 1
        self.db.get_or_create_session(session1_id, user_id=user_id)
        session1_state = {
            "user:coding_language": "Go",
            "session_goal": "Learn Cosm AST SCM"
        }
        self.db.save_combined_state(session1_id, user_id, session1_state)

        # Verify Session 1 loaded state
        loaded1 = self.db.load_combined_state(session1_id, user_id)
        self.assertEqual(loaded1.get("user:coding_language"), "Go")
        self.assertEqual(loaded1.get("session_goal"), "Learn Cosm AST SCM")

        # 2. Setup Session 2 for the same user
        self.db.get_or_create_session(session2_id, user_id=user_id)
        loaded2 = self.db.load_combined_state(session2_id, user_id)

        # Cross-session assertion:
        # User memory MUST be preserved
        self.assertEqual(loaded2.get("user:coding_language"), "Go", "User-scoped memory should persist across sessions")
        # Session scratchpad MUST NOT leak into Session 2
        self.assertIsNone(loaded2.get("session_goal"), "Session-scoped goal should NOT leak into a new session")


if __name__ == "__main__":
    unittest.main()
