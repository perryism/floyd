"""Test fixture: stamps the worker process's own PID onto the request, so a test can prove
two routes referencing this same file get independent worker pools (research.md §5t) — if
pools were incorrectly shared, two routes would report the same PID."""

import os

from gateway_handler_sdk import HttpMessage, RequestHandler


class PidHeader(RequestHandler):
    def process_request(self, request: HttpMessage) -> HttpMessage:
        request.headers["x-worker-pid"] = str(os.getpid())
        return request
