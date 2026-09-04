// Package codex implements read-oriented Codex CLI adapters for structured
// planning and review. It keeps CLI flags, prompts, schemas, and wire responses
// outside the workflow and store packages.
// Planning uses schema version 2 with an explicit implement/answer decision.
// Reviewing continues to use schema version 1. These wire versions are owned
// by the adapter; the workflow consumes validated store contracts.
package codex
