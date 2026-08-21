from __future__ import annotations

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.middleware.gzip import GZipMiddleware
from fastapi.middleware.trustedhost import TrustedHostMiddleware

from app.api import build_api_router, register_exception_handlers
from app.config import Settings, get_settings
from app.middleware import RequestIDMiddleware, RequestLoggingMiddleware, TimingMiddleware
from app.openapi import customize_openapi
from app.startup import build_lifespan


def create_app(settings: Settings | None = None) -> FastAPI:
    settings = settings or get_settings()
    print("===== cors_origins =====", settings.cors_origins, flush=True)
    print("===== RISKLENS_CORS_ORIGINS =====", __import__("os").environ.get("RISKLENS_CORS_ORIGINS"), flush=True)

    app = FastAPI(
        title=settings.app_name,
        version=settings.app_version,
        docs_url="/docs",
        redoc_url="/redoc",
        openapi_url="/openapi.json",
        lifespan=build_lifespan(settings),
    )

    _install_middleware(app, settings)
    print("===== user_middleware =====", [m.cls.__name__ for m in app.user_middleware], flush=True)
    register_exception_handlers(app)
    app.include_router(build_api_router())
    customize_openapi(app, settings)

    return app


def _install_middleware(app: FastAPI, settings: Settings) -> None:
    # add_middleware is a stack: last added runs first on the request path.
    # We want request flow:  RequestID -> RequestLogging -> Timing -> route
    # and response flow:     route -> Timing (sets duration) -> RequestLogging (logs it) -> RequestID
    # So add innermost first.
    app.add_middleware(TimingMiddleware)
    app.add_middleware(RequestLoggingMiddleware)
    app.add_middleware(RequestIDMiddleware, header_name=settings.request_id_header)

    app.add_middleware(GZipMiddleware, minimum_size=settings.gzip_min_size)

    allowed = settings.allowed_hosts or ["*"]
    app.add_middleware(TrustedHostMiddleware, allowed_hosts=allowed)

    # CORS must be outermost so Access-Control-Allow-Origin is written on
    # the final response. Last add_middleware() = first on the request path.
    print(
        "===== cors_origins truthy =====",
        bool(settings.cors_origins),
        "CORSMiddleware will be installed" if settings.cors_origins else "CORSMiddleware SKIPPED",
        flush=True,
    )
    if settings.cors_origins:
        app.add_middleware(
            CORSMiddleware,
            allow_origins=settings.cors_origins,
            allow_methods=["*"],
            allow_headers=["*"],
            allow_credentials=False,
        )


app = create_app()
