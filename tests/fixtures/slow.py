"""Test fixture: a request handler that sleeps longer than any reasonable configured
timeout, to verify the gateway's HANDLER_TIMEOUT path (as distinct from HANDLER_ERROR)."""

import time

from gateway_handler_sdk import HttpMessage, RequestHandler


class Slow(RequestHandler):
    def process_request(self, request: HttpMessage) -> HttpMessage:
        time.sleep(2)
        return request
