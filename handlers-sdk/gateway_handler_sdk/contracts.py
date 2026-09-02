"""Author-facing contracts for a gateway handler file.

A handler file (pointed to by the gateway's ``--handlers`` flag) may define a subclass of
``RequestHandler``, a subclass of ``ResponseHandler``, or both (FR-010, FR-011). The base
classes' default implementations are a no-op passthrough, so a file that only implements one
direction behaves as pass-through for the other.
"""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass
class HttpMessage:
    """Language-agnostic HTTP message, mirroring api/handler/v1/handler.proto's HttpMessage."""

    method: str = ""
    path: str = ""
    status_code: int = 0
    headers: dict[str, str] = field(default_factory=dict)
    body: bytes = b""


@dataclass
class Rejection:
    """Returned instead of an HttpMessage to reject a request or response (FR-003)."""

    error_code: str
    message: str


class RequestHandler:
    """Subclass and override ``process_request`` to inspect/modify/reject inbound requests."""

    def process_request(self, request: HttpMessage) -> HttpMessage | Rejection:
        return request


class ResponseHandler:
    """Subclass and override ``process_response`` to inspect/modify/reject responses."""

    def process_response(self, response: HttpMessage) -> HttpMessage | Rejection:
        return response
