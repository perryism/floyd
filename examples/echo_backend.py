"""Minimal stdlib-only backend for trying out examples/config.yaml: reflects the request it
received (port, path, headers) as JSON, so `curl` alone shows exactly what each route's
handler chain did to the request before it reached here — e.g. whether X-Request-Id got
added.

Usage: python3 examples/echo_backend.py <port>
"""

import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer


class Echo(BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps(
            {
                "port": self.server.server_port,
                "path": self.path,
                "headers": dict(self.headers),
            }
        ).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass  # keep example output focused on the curl responses, not per-request access logs


if __name__ == "__main__":
    port = int(sys.argv[1])
    HTTPServer(("127.0.0.1", port), Echo).serve_forever()
