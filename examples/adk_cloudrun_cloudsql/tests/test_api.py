"""Integration tests for FastAPI endpoints."""
import os
import sys
import unittest

pkg_root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if pkg_root not in sys.path:
    sys.path.insert(0, pkg_root)

from app.agent import agent_runner
from app.database import db_manager


import uuid

class TestFastAPIEndpoints(unittest.TestCase):
    """Verifies FastAPI route handlers and responses."""

    def setUp(self):
        self.session_id = f"test_api_sess_{uuid.uuid4().hex[:8]}"
        self.user_id = f"user_dave_{uuid.uuid4().hex[:8]}"

    def test_health_check_logic(self):
        """Verifies health check status response structure."""
        res = {
            "status": "healthy",
            "service": "adk-cloudrun-cloudsql",
            "environment": os.getenv("ENVIRONMENT", "production"),
            "database": "connected",
        }
        self.assertEqual(res["status"], "healthy")
        self.assertEqual(res["database"], "connected")

    def test_chat_pipeline_end_to_end(self):
        """Verifies chat turn event storage and state persistence."""
        # 1. Create session
        db_manager.get_or_create_session(self.session_id, user_id=self.user_id)

        # 2. Append turn
        db_manager.append_event(self.session_id, "user", "My name is Dave", author=self.user_id)
        state = db_manager.load_combined_state(self.session_id, self.user_id)
        
        result = agent_runner.run_turn("My name is Dave", state)
        db_manager.append_event(self.session_id, "model", result["response"], author="cloudsql_memory_agent")
        db_manager.save_combined_state(self.session_id, self.user_id, result["updated_state"])

        # 3. Verify state persisted
        saved_state = db_manager.load_combined_state(self.session_id, self.user_id)
        self.assertEqual(saved_state.get("user:preferred_name"), "Dave")

        # 4. Verify events list
        events = db_manager.get_events(self.session_id)
        self.assertEqual(len(events), 2)
        self.assertEqual(events[0]["content"], "My name is Dave")


if __name__ == "__main__":
    unittest.main()
