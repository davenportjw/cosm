"""FastAPI Application Server for Google ADK Agent on Cloud Run with Cloud SQL.

Serves the Web UI frontend, REST API, health probes, and orchestrates
the ADK agent runtime with persistent database session storage.
"""
import os
from pathlib import Path
from typing import Any, Dict, List, Optional

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import HTMLResponse, JSONResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

from .agent import agent_runner
from .database import db_manager

# Base directory paths
BASE_DIR = Path(__file__).resolve().parent
STATIC_DIR = BASE_DIR / "static"

app = FastAPI(
    title="Google ADK Cloud Run Agent",
    description="Conversational AI Agent powered by Google ADK, Gemini, Cloud Run, and Cloud SQL memory.",
    version="1.0.0",
)

# Mount static assets if directory exists
if STATIC_DIR.exists():
    app.mount("/static", StaticFiles(directory=str(STATIC_DIR)), name="static")


# Pydantic Request/Response DTOs
class ChatRequest(BaseModel):
    session_id: str
    user_id: str = "user_alice"
    message: str


class ChatResponse(BaseModel):
    session_id: str
    user_id: str
    response: str
    tool_calls: List[Dict[str, Any]]
    state: Dict[str, Any]


class SessionListResponse(BaseModel):
    sessions: List[Dict[str, Any]]


class StateResponse(BaseModel):
    session_id: str
    user_id: str
    state: Dict[str, Any]


@app.get("/", response_class=HTMLResponse)
async def get_index():
    """Serves the interactive Web UI."""
    index_file = STATIC_DIR / "index.html"
    if index_file.exists():
        with open(index_file, "r", encoding="utf-8") as f:
            return HTMLResponse(content=f.read())
    return HTMLResponse("<h1>Google ADK Agent is running</h1><p>Web UI file not found.</p>")


@app.get("/healthz")
@app.get("/healthz/")
@app.get("/health")
async def health_check():
    """Cloud Run container liveness and readiness probe."""
    return {
        "status": "healthy",
        "service": "adk-cloudrun-cloudsql",
        "environment": os.getenv("ENVIRONMENT", "production"),
        "database": "connected",
    }


@app.post("/api/chat", response_model=ChatResponse)
async def chat_turn(req: ChatRequest):
    """Executes an ADK agent turn with persistent Cloud SQL memory."""
    if not req.message.strip():
        raise HTTPException(status_code=400, detail="Message cannot be empty")

    # 1. Ensure session exists in Cloud SQL
    db_manager.get_or_create_session(
        session_id=req.session_id,
        user_id=req.user_id,
        title=req.message[:30] + ("..." if len(req.message) > 30 else "")
    )

    # 2. Record incoming user message in event history
    db_manager.append_event(
        session_id=req.session_id,
        role="user",
        author=req.user_id,
        content=req.message,
    )

    # 3. Load active state (user-scoped + session-scoped)
    state = db_manager.load_combined_state(
        session_id=req.session_id,
        user_id=req.user_id,
    )

    # 4. Run ADK agent turn (Gemini + tools)
    result = agent_runner.run_turn(
        user_message=req.message,
        current_state=state,
    )

    agent_reply = result["response"]
    tool_calls = result["tool_calls"]
    updated_state = result["updated_state"]

    # 5. Record agent response and tool executions in event history
    db_manager.append_event(
        session_id=req.session_id,
        role="model",
        author="cloudsql_memory_agent",
        content=agent_reply,
        tool_calls=tool_calls,
    )

    # 6. Save updated state back to Cloud SQL
    db_manager.save_combined_state(
        session_id=req.session_id,
        user_id=req.user_id,
        state_dict=updated_state,
    )

    return ChatResponse(
        session_id=req.session_id,
        user_id=req.user_id,
        response=agent_reply,
        tool_calls=tool_calls,
        state=updated_state,
    )


@app.get("/api/sessions", response_model=SessionListResponse)
async def list_sessions(user_id: Optional[str] = None):
    """Retrieves all sessions from Cloud SQL."""
    sessions = db_manager.list_sessions(user_id=user_id)
    return SessionListResponse(sessions=sessions)


@app.get("/api/sessions/{session_id}/events")
async def get_session_events(session_id: str):
    """Retrieves full conversation event history for a session."""
    events = db_manager.get_events(session_id=session_id)
    return {"session_id": session_id, "events": events}


@app.get("/api/sessions/{session_id}/state", response_model=StateResponse)
async def get_session_state(session_id: str, user_id: str = "user_alice"):
    """Retrieves combined user and session state from Cloud SQL."""
    state = db_manager.load_combined_state(session_id=session_id, user_id=user_id)
    return StateResponse(session_id=session_id, user_id=user_id, state=state)


if __name__ == "__main__":
    import uvicorn
    port = int(os.environ.get("PORT", 8080))
    host = os.environ.get("HOST", "0.0.0.0")
    print(f"Starting Google ADK Agent Server on http://{host}:{port}")
    uvicorn.run("app.main:app", host=host, port=port, reload=True)
