"""US1 acceptance scenario 1: a request handler that adds a header to every request."""

from gateway_handler_sdk import HttpMessage, RequestHandler


class AddHeader(RequestHandler):
    def process_request(self, request: HttpMessage) -> HttpMessage:
        request.headers["x-injected"] = "yes"
        return request
