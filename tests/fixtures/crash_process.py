"""Unit test fixture: hard-exits the whole worker process (not just raising a Python
exception, which gRPC would catch and turn into an RPC error without killing the process).
Used to verify workerpool's crash-detection/restart/degrade state machine."""

import os

from gateway_handler_sdk import HttpMessage, RequestHandler


class CrashProcess(RequestHandler):
    def process_request(self, request: HttpMessage) -> HttpMessage:
        os._exit(1)
