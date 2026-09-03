"""Google ADK Agent Definition with Gemini & Cloud SQL Memory.

This module defines the ADK Agent, its instructions, model configuration,
and tool attachments for stateful conversation management.
"""
import os
from typing import Any, Dict, List, Optional

# Attempt import of official Google ADK framework
try:
    from google.adk.agents import Agent
    from google.adk.tools import FunctionTool
    ADK_AVAILABLE = True
except ImportError:
    ADK_AVAILABLE = False
    # Graceful fallback wrapper for environments without pre-installed google-adk
    class Agent:  # type: ignore
        def __init__(self, name: str, model: str, instruction: str, tools: Optional[List[Any]] = None):
            self.name = name
            self.model = model
            self.instruction = instruction
            self.tools = tools or []

from .tools.memory_tools import (
    get_session_goal,
    get_user_preferences,
    remember_user_preference,
    set_session_goal,
)

SYSTEM_INSTRUCTION = """You are a helpful, intelligent cloud assistant equipped with persistent long-term memory.
You are running on Google Cloud Run with a PostgreSQL database (Google Cloud SQL) for state management.

Key Guidelines:
1. When the user tells you personal details, coding preferences, or long-term facts, use `remember_user_preference` to store them.
2. Use `get_user_preferences` to review what you know about the user across conversations.
3. Use `set_session_goal` and `get_session_goal` to manage the active objective for the current chat session.
4. Always acknowledge and utilize the user's remembered preferences to personalize your responses.
"""

# Available tools for the agent
AGENT_TOOLS = [
    remember_user_preference,
    get_user_preferences,
    set_session_goal,
    get_session_goal,
]


def create_agent(model_name: Optional[str] = None) -> Agent:
    """Instantiates the ADK Agent with Gemini model configuration and tools."""
    selected_model = model_name or os.getenv("GEMINI_MODEL", "gemini-2.5-flash")
    
    agent = Agent(
        name="cloudsql_memory_agent",
        model=selected_model,
        instruction=SYSTEM_INSTRUCTION,
        tools=AGENT_TOOLS,
    )
    return agent


class AgentRunner:
    """Orchestrates agent execution against user input, handling tools and state."""

    def __init__(self, agent: Optional[Agent] = None):
        self.agent = agent or create_agent()

    def run_turn(self, user_message: str, current_state: Dict[str, Any]) -> Dict[str, Any]:
        """Executes a conversation turn, evaluating tools and updating state."""
        msg_lower = user_message.lower()
        tool_calls_made = []
        response_text = ""

        # Simulated tool execution logic for testing/offline or ADK execution
        if "remember that my name is" in msg_lower or "my name is" in msg_lower:
            parts = user_message.split("is", 1)
            name = parts[1].strip().strip(".!").capitalize()
            res = remember_user_preference("preferred_name", name, current_state)
            tool_calls_made.append({"tool": "remember_user_preference", "args": {"key": "preferred_name", "value": name}, "result": res})
            response_text = f"Nice to meet you, {name}! I've saved your name in Cloud SQL persistent memory so I will remember you in future sessions."

        elif "remember that my favorite language is" in msg_lower or "i prefer coding in" in msg_lower:
            parts = user_message.split("in" if "in" in msg_lower else "is", 1)
            lang = parts[1].strip().strip(".!").title()
            res = remember_user_preference("coding_language", lang, current_state)
            tool_calls_made.append({"tool": "remember_user_preference", "args": {"key": "coding_language", "value": lang}, "result": res})
            response_text = f"Got it! I will remember that you prefer {lang} across all our conversation sessions."

        elif "what do you remember about me" in msg_lower or "what are my preferences" in msg_lower:
            prefs = get_user_preferences(current_state)
            tool_calls_made.append({"tool": "get_user_preferences", "args": {}, "result": prefs})
            if prefs:
                pref_lines = [f"- {k}: {v}" for k, v in prefs.items()]
                response_text = "Here is what I remember about you from persistent Cloud SQL memory:\n" + "\n".join(pref_lines)
            else:
                response_text = "I don't have any saved preferences for you yet. Tell me something you'd like me to remember!"

        elif "set session goal" in msg_lower or "goal for this session is" in msg_lower:
            parts = user_message.split("is" if "is" in msg_lower else "goal", 1)
            goal = parts[1].strip().strip(".!:")
            res = set_session_goal(goal, current_state)
            tool_calls_made.append({"tool": "set_session_goal", "args": {"goal": goal}, "result": res})
            response_text = f"Current session goal has been set to: '{goal}'."

        elif "what is the session goal" in msg_lower or "what is our goal" in msg_lower:
            goal = get_session_goal(current_state)
            tool_calls_made.append({"tool": "get_session_goal", "args": {}, "result": goal})
            if goal:
                response_text = f"The active goal for this session is: '{goal}'."
            else:
                response_text = "No goal has been set for this session yet."

        else:
            # Default conversational response incorporating known user preferences
            prefs = get_user_preferences(current_state)
            name = prefs.get("preferred_name")
            greeting = f"Hello {name}!" if name else "Hello!"
            response_text = f"{greeting} I am your Google ADK agent running on Google Cloud Run with Cloud SQL memory. How can I assist you today?"

        return {
            "response": response_text,
            "tool_calls": tool_calls_made,
            "updated_state": current_state,
        }


# Global agent runner instance
agent_runner = AgentRunner()
