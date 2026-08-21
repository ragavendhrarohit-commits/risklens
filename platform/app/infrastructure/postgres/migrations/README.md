# Migrations

Alembic owns the `agents` table schema. The connection string is resolved by
`env.py` from application `Settings`, so migrations and the running service
always target the same database.

```powershell
cd platform

# Apply everything
alembic upgrade head

# Roll back one revision
alembic downgrade -1

# Inspect
alembic current
alembic history --verbose

# Target a specific database without touching .env
alembic -x url=postgresql+psycopg://user:pass@host:5432/dbname upgrade head

# After changing models.py, generate the next revision
alembic revision --autogenerate -m "describe the change"
```

Always read a generated revision before committing it. Autogenerate does not
detect renames, server-default changes on some types, or data migrations.
