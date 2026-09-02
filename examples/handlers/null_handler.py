"""A no-op handler: passes every request and response through unmodified.

Useful as a template to start customizing, or as an explicit placeholder when you want
--handlers to point at something concrete rather than omitting the flag (omitting --handlers
entirely gives you the same pass-through behavior with no worker process at all).
"""

from gateway_handler_sdk import HttpMessage, RequestHandler, ResponseHandler


class NullRequestHandler(RequestHandler):
    def process_request(self, request: HttpMessage) -> HttpMessage:
        return request


class NullResponseHandler(ResponseHandler):
    def process_response(self, response: HttpMessage) -> HttpMessage:
        return response
