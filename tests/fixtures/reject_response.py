"""Contract test fixture: a response handler that always rejects, to verify the
ProcessResponse rejection shape."""

from gateway_handler_sdk import HttpMessage, Rejection, ResponseHandler


class RejectResponse(ResponseHandler):
    def process_response(self, response: HttpMessage):
        return Rejection("BLOCKED_RESPONSE", "this response is blocked for testing")
