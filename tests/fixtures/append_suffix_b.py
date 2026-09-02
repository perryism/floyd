"""US3 acceptance scenario 1: second of two chained request handlers; appends "-B" to
x-order. If chain order is correct, the final value is "-A-B" (this handler saw the first's
modification)."""

from gateway_handler_sdk import HttpMessage, RequestHandler


class AppendSuffixB(RequestHandler):
    def process_request(self, request: HttpMessage) -> HttpMessage:
        request.headers["x-order"] = request.headers.get("x-order", "") + "-B"
        return request
