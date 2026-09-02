"""Boilerplate gRPC worker entrypoint. Handler authors never import this module directly —
the gateway launches it as ``python3 -m gateway_handler_sdk.server --handler-file <path>
--socket <path>`` (see internal/handler/workerpool/pool.go).

It loads the author's handler file, discovers whichever of RequestHandler/ResponseHandler it
defines, and serves the Handler gRPC service (api/handler/v1/handler.proto) over a Unix domain
socket, translating between the wire protobuf messages and the SDK's friendly HttpMessage.
"""

from __future__ import annotations

import argparse
import importlib.util
import inspect
import sys
from concurrent import futures

import grpc

from .contracts import HttpMessage, Rejection, RequestHandler, ResponseHandler
from .v1 import handler_pb2, handler_pb2_grpc


def _load_handler_module(handler_file: str):
    spec = importlib.util.spec_from_file_location("gateway_handler_module", handler_file)
    if spec is None or spec.loader is None:
        raise ImportError(f"could not load handler file: {handler_file}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _find_instance(module, base_class):
    """Return the first instance of a base_class subclass defined in module, or None.

    A handler file MAY implement a request handler, a response handler, or both (FR-010,
    FR-011) — this picks whichever base classes it subclasses.
    """
    for _, obj in inspect.getmembers(module, inspect.isclass):
        if obj is base_class:
            continue
        if issubclass(obj, base_class) and obj.__module__ == module.__name__:
            return obj()
    return None


def _to_message(msg: handler_pb2.HttpMessage) -> HttpMessage:
    return HttpMessage(
        method=msg.method,
        path=msg.path,
        status_code=msg.status_code,
        headers=dict(msg.headers),
        body=msg.body,
    )


def _from_result(result) -> handler_pb2.ProcessRequestResp | None:
    """Shared by ProcessRequest/ProcessResponse; caller wraps the oneof appropriately."""
    if isinstance(result, Rejection):
        return handler_pb2.Rejection(error_code=result.error_code, message=result.message)
    return handler_pb2.HttpMessage(
        method=result.method,
        path=result.path,
        status_code=result.status_code,
        headers=result.headers,
        body=result.body,
    )


class HandlerServicer(handler_pb2_grpc.HandlerServicer):
    def __init__(self, request_handler: RequestHandler | None, response_handler: ResponseHandler | None):
        self._request_handler = request_handler or RequestHandler()
        self._response_handler = response_handler or ResponseHandler()

    def ProcessRequest(self, request, context):
        result = self._request_handler.process_request(_to_message(request.request))
        wire = _from_result(result)
        if isinstance(result, Rejection):
            return handler_pb2.ProcessRequestResp(rejection=wire)
        return handler_pb2.ProcessRequestResp(modified_request=wire)

    def ProcessResponse(self, request, context):
        result = self._response_handler.process_response(_to_message(request.response))
        wire = _from_result(result)
        if isinstance(result, Rejection):
            return handler_pb2.ProcessResponseResp(rejection=wire)
        return handler_pb2.ProcessResponseResp(modified_response=wire)


def serve(handler_file: str, socket_path: str) -> None:
    module = _load_handler_module(handler_file)
    request_handler = _find_instance(module, RequestHandler)
    response_handler = _find_instance(module, ResponseHandler)

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=4))
    handler_pb2_grpc.add_HandlerServicer_to_server(
        HandlerServicer(request_handler, response_handler), server
    )
    server.add_insecure_port(f"unix:{socket_path}")
    server.start()
    server.wait_for_termination()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--handler-file", required=True)
    parser.add_argument("--socket", required=True)
    args = parser.parse_args()
    serve(args.handler_file, args.socket)


if __name__ == "__main__":
    sys.exit(main())
