from __future__ import annotations

from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.requests import Request
from starlette.responses import Response

from app.logging.setup import get_logger

_logger = get_logger("risklens.request")


class RequestLoggingMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next: RequestResponseEndpoint) -> Response:
        _logger.info(
            "request received",
            extra={
                "method": request.method,
                "path": request.url.path,
                "client": request.client.host if request.client else None,
            },
        )
        try:
            response = await call_next(request)
        except Exception:
            _logger.exception(
                "request failed",
                extra={"method": request.method, "path": request.url.path},
            )
            raise
        duration = getattr(request.state, "duration_ms", None)
        _logger.info(
            "request completed",
            extra={
                "method": request.method,
                "path": request.url.path,
                "status_code": response.status_code,
                "duration_ms": duration,
            },
        )
        return response
