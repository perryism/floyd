"""US3 acceptance scenario 2: a request handler that always raises, to verify the gateway
fails closed and keeps serving other requests (FR-006, FR-012)."""

from gateway_handler_sdk import HttpMessage, RequestHandler


class AlwaysFail(RequestHandler):
    def process_request(self, request: HttpMessage) -> HttpMessage:
        raise RuntimeError("intentional failure for testing")
