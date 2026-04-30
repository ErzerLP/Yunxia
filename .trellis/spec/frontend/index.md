# Frontend Development Guidelines

> Project-specific frontend conventions for Yunxia.

## Overview

The frontend lives in `web/` and uses React, TypeScript, Vite, Tailwind CSS, Axios, TanStack Query, Zustand, React Router, and Lucide icons. Trust `web/package.json` over older planning docs for installed versions.

Primary sources:
1. Current code in `web/src/**`
2. `backend/FRONTEND_HANDOFF.md` for the current backend-to-frontend adaptation queue, status, and checklist
3. `backend/API_CONTRACT.md` for API shape, DTOs, errors, and permissions
4. `web/scripts/check-vfs-integration.mjs` for regression invariants
5. `FRONTEND-DESIGN.md`, `FRONTEND-PLAN.md`, `docs/frontend-api-roadmap.md`, `docs/frontend-progress.md` for design/history; some may be stale

## Guidelines Index

| Guide | Description | Status |
|---|---|---|
| [Directory Structure](./directory-structure.md) | Source layout, routing, API module placement | Filled |
| [Component Guidelines](./component-guidelines.md) | Component shape, props, Tailwind/cn, accessibility | Filled |
| [Hook Guidelines](./hook-guidelines.md) | Custom hooks, guards, React Query patterns | Filled |
| [State Management](./state-management.md) | Zustand vs React Query vs local state | Filled |
| [Type Safety](./type-safety.md) | API types, strict TS, unknown catches | Filled |
| [Quality Guidelines](./quality-guidelines.md) | Build/lint/static checks, review checklist | Filled |

## Pre-Development Checklist

Before frontend coding, read:

- This index
- `directory-structure.md`
- `component-guidelines.md`
- `state-management.md`
- `type-safety.md`
- `quality-guidelines.md`
- `hook-guidelines.md` when adding hooks, guards, or query logic
- `backend/FRONTEND_HANDOFF.md` when adapting backend-facing features/interfaces or checking current frontend TODOs
- `backend/API_CONTRACT.md` when changing API clients or DTO types

## Language

Spec docs are English. Current UI labels are Chinese and should stay consistent unless the task says otherwise.
