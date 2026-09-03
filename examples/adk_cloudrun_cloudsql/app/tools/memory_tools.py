"""Memory and State Management Tools for Google ADK Agent.

These tools allow the AI agent to explicitly read and write:
1. Session-scoped state (temporary context within a single chat session).
2. User-scoped memory (long-term preferences and facts that persist across sessions via the 'user:' prefix).
"""
from typing import Any, Dict, Optional


def remember_user_preference(key: str, value: str, tool_context: Optional[Any] = None) -> str:
    """Stores a long-term preference or fact about the user.
    
    This information persists across all future conversation sessions for this user.
    
    Args:
        key: The attribute or preference name (e.g., 'coding_language', 'preferred_name', 'skill_level').
        value: The value to remember (e.g., 'Python', 'Alice', 'Advanced').
        tool_context: ADK ToolContext providing state access.
        
    Returns:
        Confirmation message that the preference was saved.
    """
    state_key = f"user:{key.lower().strip()}"
    
    if tool_context is not None and hasattr(tool_context, "state"):
        tool_context.state[state_key] = value
        return f"Successfully saved user preference: '{key}' = '{value}'"
    elif tool_context is not None and isinstance(tool_context, dict):
        tool_context[state_key] = value
        return f"Successfully saved user preference: '{key}' = '{value}'"
    
    return f"Simulated save of user preference: '{key}' = '{value}' (no active ToolContext)"


def get_user_preferences(tool_context: Optional[Any] = None) -> Dict[str, Any]:
    """Retrieves all remembered long-term facts and preferences for the current user.
    
    Args:
        tool_context: ADK ToolContext providing state access.
        
    Returns:
        Dictionary of all user-scoped preferences.
    """
    preferences: Dict[str, Any] = {}
    
    if tool_context is not None and hasattr(tool_context, "state"):
        for k, v in tool_context.state.items():
            if k.startswith("user:"):
                clean_key = k[len("user:"):]
                preferences[clean_key] = v
    elif tool_context is not None and isinstance(tool_context, dict):
        for k, v in tool_context.items():
            if k.startswith("user:"):
                clean_key = k[len("user:"):]
                preferences[clean_key] = v
                
    return preferences


def set_session_goal(goal: str, tool_context: Optional[Any] = None) -> str:
    """Sets the active objective or task goal for the current conversation session.
    
    This goal is session-scoped and will be cleared when a new session begins.
    
    Args:
        goal: The goal description (e.g., 'Build a Cloud SQL integration tutorial').
        tool_context: ADK ToolContext providing state access.
        
    Returns:
        Confirmation message.
    """
    if tool_context is not None and hasattr(tool_context, "state"):
        tool_context.state["session_goal"] = goal
        return f"Session goal updated to: '{goal}'"
    elif tool_context is not None and isinstance(tool_context, dict):
        tool_context["session_goal"] = goal
        return f"Session goal updated to: '{goal}'"
        
    return f"Simulated session goal update: '{goal}'"


def get_session_goal(tool_context: Optional[Any] = None) -> Optional[str]:
    """Retrieves the active session goal if one was previously set.
    
    Args:
        tool_context: ADK ToolContext providing state access.
        
    Returns:
        The current session goal, or None if not set.
    """
    if tool_context is not None and hasattr(tool_context, "state"):
        return tool_context.state.get("session_goal")
    elif tool_context is not None and isinstance(tool_context, dict):
        return tool_context.get("session_goal")
    return None
