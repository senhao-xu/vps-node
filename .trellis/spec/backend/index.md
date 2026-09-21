# Backend Development Guidelines

> Best practices for backend development in this project.

---

## Overview

This directory contains guidelines for backend development. Fill in each file with your project's specific conventions.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | Module organization and file layout | Filled |
| [Database Guidelines](./database-guidelines.md) | SQLite, migrations, revision bump + batch idempotency invariants | Filled |
| [Error Handling](./error-handling.md) | Error envelope, validation matrix, auth boundary, secrets | Filled |
| [Node Protocol Settings](./node-protocol-settings.md) | Reality generation, protocol validation, settings patch semantics | Filled |
| [Quality Guidelines](./quality-guidelines.md) | Code standards, forbidden patterns, validation commands | Filled |
| [Deploy Guidelines](./deploy-guidelines.md) | Docker/systemd deployment, container zombie gotcha, secrets handling | Filled |
| [Logging Guidelines](./logging-guidelines.md) | Structured logging, log levels | To fill |

---

## How to Fill These Guidelines

For each guideline file:

1. Document your project's **actual conventions** (not ideals)
2. Include **code examples** from your codebase
3. List **forbidden patterns** and why
4. Add **common mistakes** your team has made

The goal is to help AI assistants and new team members understand how YOUR project works.

---

**Language**: All documentation should be written in **English**.
