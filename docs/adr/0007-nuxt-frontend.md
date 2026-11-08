# ADR 0007: Nuxt 4 frontend with English routes

Status: accepted 2026-11-08.

## Context

Danny builds aboutme.vn with Nuxt 4, Vue 3, Pinia, reka-ui, Tailwind and an OpenAPI-generated client, and wants the same stack and components here.

## Decision

apps/web is Nuxt 4 SSR with that stack. Routes are English with a country segment (/vn/search, /vn/documents/{key}, /vn/topics/{code}, /connect, /sources, /account). Interface text defaults to Vietnamese with optional English. Public pages are cached by Nitro route rules.

## Consequences

Reuses known patterns and components. Adds a Node process of about 200 MB and a build step to the host.

## Alternatives rejected

Go templates with htmx (least code, but no reuse). A separate Next.js service. Vietnamese route slugs (conflict with multi-country URLs).

## History

- 2026-11-08: accepted in the brainstorming conversation.
