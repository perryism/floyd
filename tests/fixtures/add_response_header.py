"""US2 acceptance scenario 2: a response handler that adds a header to the response."""

from gateway_handler_sdk import HttpMessage, ResponseHandler


class AddResponseHeader(ResponseHandler):
    def process_response(self, response: HttpMessage) -> HttpMessage:
        response.headers["x-cache"] = "miss"
        return response
