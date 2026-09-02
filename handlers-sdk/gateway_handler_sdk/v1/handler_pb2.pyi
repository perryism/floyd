from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class HttpMessage(_message.Message):
    __slots__ = ("method", "path", "status_code", "headers", "body")
    class HeadersEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    METHOD_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    STATUS_CODE_FIELD_NUMBER: _ClassVar[int]
    HEADERS_FIELD_NUMBER: _ClassVar[int]
    BODY_FIELD_NUMBER: _ClassVar[int]
    method: str
    path: str
    status_code: int
    headers: _containers.ScalarMap[str, str]
    body: bytes
    def __init__(self, method: _Optional[str] = ..., path: _Optional[str] = ..., status_code: _Optional[int] = ..., headers: _Optional[_Mapping[str, str]] = ..., body: _Optional[bytes] = ...) -> None: ...

class Rejection(_message.Message):
    __slots__ = ("error_code", "message")
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    error_code: str
    message: str
    def __init__(self, error_code: _Optional[str] = ..., message: _Optional[str] = ...) -> None: ...

class ProcessRequestReq(_message.Message):
    __slots__ = ("request_id", "request")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    REQUEST_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    request: HttpMessage
    def __init__(self, request_id: _Optional[str] = ..., request: _Optional[_Union[HttpMessage, _Mapping]] = ...) -> None: ...

class ProcessRequestResp(_message.Message):
    __slots__ = ("modified_request", "rejection")
    MODIFIED_REQUEST_FIELD_NUMBER: _ClassVar[int]
    REJECTION_FIELD_NUMBER: _ClassVar[int]
    modified_request: HttpMessage
    rejection: Rejection
    def __init__(self, modified_request: _Optional[_Union[HttpMessage, _Mapping]] = ..., rejection: _Optional[_Union[Rejection, _Mapping]] = ...) -> None: ...

class ProcessResponseReq(_message.Message):
    __slots__ = ("request_id", "response")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    RESPONSE_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    response: HttpMessage
    def __init__(self, request_id: _Optional[str] = ..., response: _Optional[_Union[HttpMessage, _Mapping]] = ...) -> None: ...

class ProcessResponseResp(_message.Message):
    __slots__ = ("modified_response", "rejection")
    MODIFIED_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    REJECTION_FIELD_NUMBER: _ClassVar[int]
    modified_response: HttpMessage
    rejection: Rejection
    def __init__(self, modified_response: _Optional[_Union[HttpMessage, _Mapping]] = ..., rejection: _Optional[_Union[Rejection, _Mapping]] = ...) -> None: ...
