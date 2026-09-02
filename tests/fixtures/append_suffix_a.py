"""US3 acceptance scenario 1: first of two chained request handlers; appends "-A" to the
x-order header (creating it if absent) so a test can verify chain order."""

from gateway_handler_sdk import HttpMessage, RequestHandler


class AppendSuffixA(RequestHandler):
    def process_request(self, request: HttpMessage) -> HttpMessage:
        request.headers["x-order"] = request.headers.get("x-order", "") + "-A"
        return request
