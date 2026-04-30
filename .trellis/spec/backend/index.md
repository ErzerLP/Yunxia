# Backend Development Guidelines

> Project-specific backend conventions for Yunxia.

## Overview

The backend lives in `backend/` and is a Go service using Gin, GORM/SQLite, JWT, `log/slog`, and DDD-style four-layer architecture. These specs describe current code, not future ideals.

Primary sources:
1. Current code in `backend/internal/**`
2. `backend/API_CONTRACT.md` for REST/WebDAV API contract
3. `DOCS-INDEX.md` and archived design docs under `docs/archive/2026-04-initial-design/` for architecture intent

## Guidelines Index

| Guide | Description | Status |
|---|---|---|
| [Directory Structure](./directory-structure.md) | DDD layers, dependency direction, feature placement | Filled |
| [Database Guidelines](./database-guidelines.md) | GORM models, AutoMigrate, repository conventions | Filled |
| [Error Handling](./error-handling.md) | Sentinel errors, response envelope, handler mapping | Filled |
| [Logging Guidelines](./logging-guidelines.md) | `slog`, access log, request IDs, audit logging | Filled |
| [Quality Guidelines](./quality-guidelines.md) | Backend review checklist, tests, forbidden patterns, frontend handoff/index maintenance | Filled |

## Pre-Development Checklist

Before backend coding, read:

- This index
- `directory-structure.md`
- `error-handling.md`
- `quality-guidelines.md`
- `database-guidelines.md` when touching persistence, entities, repositories, migrations/AutoMigrate, upload/task/share/audit stored fields
- `logging-guidelines.md` when touching middleware, audit, background workers, or operational events
- `backend/API_CONTRACT.md` when changing any route, DTO, response field, error code, or frontend-facing behavior
- For frontend-facing backend changes, also prepare frontend handoff notes in `backend/API_CONTRACT.md`: purpose, routes, payloads, response shape, errors, permissions, and recommended UI flow. Use `backend/FRONTEND_HANDOFF.md` as the fixed handoff document. Maintain its top `待适配索引`, fixed status values, stable anchors, and per-entry frontend checklist; append detailed updates at the end instead of creating one-off docs. When adding a new document, update the relevant index document when needed so the document remains discoverable.

## Language

Spec docs are English. Existing Go comments are concise Chinese and may remain that way for consistency.



