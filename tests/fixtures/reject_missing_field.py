"""US1 acceptance scenario 2: a request handler that rejects requests missing a required
header."""

from gateway_handler_sdk import HttpMessage, Rejection, RequestHandler


class RejectMissingField(RequestHandler):
    def process_request(self, request: HttpMessage):
        if "x-required" not in request.headers:
            return Rejection("MISSING_FIELD", "x-required header is missing")
        return request
