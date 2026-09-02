"""Reference handler from quickstart.md — a minimal request handler that stamps every
request with an X-Request-Id header if the caller didn't already send one."""

import uuid

from gateway_handler_sdk import HttpMessage, RequestHandler


class AddRequestIdHandler(RequestHandler):
    def process_request(self, request: HttpMessage) -> HttpMessage:
        if "x-request-id" not in request.headers:
            request.headers["X-Request-Id"] = str(uuid.uuid4())
        return request
