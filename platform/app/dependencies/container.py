from __future__ import annotations

from threading import RLock
from typing import Any, Callable, TypeVar, cast

from fastapi import Depends, Request

from app.config import Settings, get_settings

T = TypeVar("T")


class DependencyNotRegistered(KeyError):
    pass


class Container:
    """Simple dependency container.

    Services register themselves as either instances or factories.
    Sprint 1 ships with only Settings. Future sprints add repositories,
    services, and clients by calling `register` in the startup path.
    """

    def __init__(self, settings: Settings) -> None:
        self._settings = settings
        self._instances: dict[type, Any] = {}
        self._factories: dict[type, Callable[["Container"], Any]] = {}
        self._lock = RLock()
        self.register_instance(Settings, settings)

    @property
    def settings(self) -> Settings:
        return self._settings

    def register_instance(self, key: type[T], instance: T) -> None:
        with self._lock:
            self._instances[key] = instance

    def register_factory(self, key: type[T], factory: Callable[["Container"], T]) -> None:
        with self._lock:
            self._factories[key] = factory

    def resolve(self, key: type[T]) -> T:
        with self._lock:
            if key in self._instances:
                return cast(T, self._instances[key])
            factory = self._factories.get(key)
            if factory is None:
                raise DependencyNotRegistered(key)
            instance = factory(self)
            self._instances[key] = instance
            return cast(T, instance)

    def has(self, key: type) -> bool:
        with self._lock:
            return key in self._instances or key in self._factories

    def close(self) -> None:
        with self._lock:
            self._instances.clear()
            self._factories.clear()


def get_container(request: Request) -> Container:
    container = getattr(request.app.state, "container", None)
    if container is None:
        raise RuntimeError("Dependency container is not initialized")
    return container


def get_settings_dep(container: Container = Depends(get_container)) -> Settings:
    return container.resolve(Settings)
