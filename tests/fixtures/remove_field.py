"""US2 acceptance scenario 1: a response handler that removes a field from the backend's JSON
response body."""

import json

from gateway_handler_sdk import HttpMessage, ResponseHandler


class RemoveField(ResponseHandler):
    def process_response(self, response: HttpMessage) -> HttpMessage:
        data = json.loads(response.body)
        data.pop("secret", None)
        response.body = json.dumps(data).encode()
        return response
