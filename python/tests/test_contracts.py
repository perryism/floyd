from gateway_handler_sdk import HttpMessage, Rejection, RequestHandler, ResponseHandler


def test_request_handler_default_is_passthrough():
    msg = HttpMessage(method="GET", path="/x", headers={"a": "b"}, body=b"hi")
    result = RequestHandler().process_request(msg)
    assert result is msg


def test_response_handler_default_is_passthrough():
    msg = HttpMessage(status_code=200, body=b"hi")
    result = ResponseHandler().process_response(msg)
    assert result is msg


def test_http_message_defaults():
    msg = HttpMessage()
    assert msg.method == ""
    assert msg.path == ""
    assert msg.status_code == 0
    assert msg.headers == {}
    assert msg.body == b""


def test_rejection_requires_code_and_message():
    rejection = Rejection("SOME_CODE", "some message")
    assert rejection.error_code == "SOME_CODE"
    assert rejection.message == "some message"
