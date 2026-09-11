"""A local OpenAI-compatible server for tests that need a real HTTP round trip."""

from __future__ import annotations

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any


class Server:
    """Serves on 127.0.0.1 and answers a POST to a path in `replies` with that reply as JSON.

    Every request is appended to `requests` as (method, path). A CONNECT is recorded and refused,
    so a process whose HTTP(S) proxy is this server records requests bound for any other host.
    """

    def __init__(self, replies: dict[str, Any]) -> None:
        self.replies = replies
        self.requests: list[tuple[str, str]] = []
        requests = self.requests

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self) -> None:
                requests.append(("POST", self.path))
                self.rfile.read(int(self.headers.get("Content-Length") or 0))
                body = json.dumps(replies[self.path]).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def do_CONNECT(self) -> None:
                requests.append(("CONNECT", self.path))
                self.send_error(403)

            def log_message(self, format: str, *args: Any) -> None:
                pass

        self._server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self._server.server_address[1]}"

    def __enter__(self) -> Server:
        threading.Thread(target=self._server.serve_forever, daemon=True).start()
        return self

    def __exit__(self, *exc: object) -> None:
        self._server.shutdown()
        self._server.server_close()
