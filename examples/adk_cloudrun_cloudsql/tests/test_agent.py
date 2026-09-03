"""Unit tests for Google ADK Agent creation and tool execution."""
import os
import sys
import unittest

pkg_root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if pkg_root not in sys.path:
    sys.path.insert(0, pkg_root)

from app.agent import AgentRunner, create_agent, SYSTEM_INSTRUCTION
from app.tools.memory_tools import (
    get_session_goal,
    get_user_preferences,
    remember_user_preference,
    set_session_goal,
)


class TestADKAgent(unittest.TestCase):
    """Tests ADK agent definitions, instructions, and memory tools."""

    def setUp(self):
        self.agent = create_agent(model_name="gemini-2.5-flash")
        self.runner = AgentRunner(self.agent)

    def test_agent_initialization(self):
        """Verifies that the agent initializes with correct metadata and tools."""
        self.assertEqual(self.agent.name, "cloudsql_memory_agent")
        self.assertEqual(self.agent.model, "gemini-2.5-flash")
        self.assertIn("persistent long-term memory", self.agent.instruction)
        self.assertEqual(len(self.agent.tools), 4)

    def test_remember_user_preference_tool(self):
        """Verifies that user memory tool stores prefixed user:* keys."""
        state = {}
        res = remember_user_preference("favorite_color", "blue", state)
        self.assertIn("Successfully saved user preference", res)
        self.assertEqual(state.get("user:favorite_color"), "blue")

        # Test retrieval
        prefs = get_user_preferences(state)
        self.assertEqual(prefs.get("favorite_color"), "blue")

    def test_session_goal_tool(self):
        """Verifies that session goal tool updates session-scoped state."""
        state = {}
        res = set_session_goal("Deploy to Cloud Run", state)
        self.assertIn("Session goal updated", res)
        self.assertEqual(state.get("session_goal"), "Deploy to Cloud Run")

        # Test retrieval
        goal = get_session_goal(state)
        self.assertEqual(goal, "Deploy to Cloud Run")

    def test_agent_turn_execution(self):
        """Verifies that the agent runner processes turns and updates memory."""
        state = {}
        
        # Turn 1: User introduces themselves
        turn1 = self.runner.run_turn("My name is Alice", state)
        self.assertIn("Alice", turn1["response"])
        self.assertEqual(state.get("user:preferred_name"), "Alice")
        self.assertTrue(any(tc["tool"] == "remember_user_preference" for tc in turn1["tool_calls"]))

        # Turn 2: User asks what the agent remembers
        turn2 = self.runner.run_turn("What do you remember about me?", state)
        self.assertIn("Alice", turn2["response"])
        self.assertTrue(any(tc["tool"] == "get_user_preferences" for tc in turn2["tool_calls"]))


if __name__ == "__main__":
    unittest.main()
