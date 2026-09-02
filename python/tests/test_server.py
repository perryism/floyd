import os

import pytest
from gateway_handler_sdk import HttpMessage, Rejection, RequestHandler, ResponseHandler
from gateway_handler_sdk.server import (
    _find_instance,
    _from_result,
    _load_handler_module,
    _to_message,
)
from gateway_handler_sdk.v1 import handler_pb2

FIXTURES = os.path.join(os.path.dirname(__file__), "..", "..", "tests", "fixtures")


def test_load_handler_module_and_find_request_handler():
    module = _load_handler_module(os.path.join(FIXTURES, "add_header.py"))
    handler = _find_instance(module, RequestHandler)
    assert handler is not None
    assert handler.process_request(HttpMessage(headers={})).headers["x-injected"] == "yes"


def test_find_instance_returns_none_when_direction_not_implemented():
    module = _load_handler_module(os.path.join(FIXTURES, "add_header.py"))
    assert _find_instance(module, ResponseHandler) is None


def test_to_message_converts_proto_to_dataclass():
    proto_msg = handler_pb2.HttpMessage(method="GET", path="/x", headers={"a": "b"}, body=b"hi")
    msg = _to_message(proto_msg)
    assert msg.method == "GET"
    assert msg.path == "/x"
    assert msg.headers == {"a": "b"}
    assert msg.body == b"hi"


def test_from_result_wraps_message():
    msg = HttpMessage(method="GET", path="/x", status_code=200, headers={"a": "b"}, body=b"hi")
    wire = _from_result(msg)
    assert isinstance(wire, handler_pb2.HttpMessage)
    assert wire.method == "GET"
    assert wire.body == b"hi"


def test_from_result_wraps_rejection():
    rejection = Rejection("SOME_CODE", "some message")
    wire = _from_result(rejection)
    assert isinstance(wire, handler_pb2.Rejection)
    assert wire.error_code == "SOME_CODE"
    assert wire.message == "some message"


def test_load_handler_module_missing_file_raises():
    with pytest.raises(Exception):
        _load_handler_module(os.path.join(FIXTURES, "does_not_exist.py"))
