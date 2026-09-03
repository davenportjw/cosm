"""Test Suite for Google ADK Agent with Cloud SQL Memory and Cloud Run WebUI."""
import os
import sys

# Ensure app package is importable regardless of working directory
pkg_root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if pkg_root not in sys.path:
    sys.path.insert(0, pkg_root)
