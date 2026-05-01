# Changelog

Brief record of user-visible changes and notable project decisions.

## Unreleased

- Implemented local SQLite FTS5 search across `/raw/*` and `/wiki/*`, including `alienshard index rebuild`, `/search`, `/search/status`, `/search/reindex`, wiki mutation index updates, and markdown link metadata.
- Added automatic MkDocs Material publishing for `docs/` through GitHub Pages on documentation-related pushes to `main`.
- Reorganized documentation so `README.md` stays an all-around quickstarter while deeper reference docs live under `docs/`.
- Added `DELETE /wiki/<path>.md` support for deleting wiki Markdown pages, including managed index refreshes.
- Added a canonical search design and resume checklist in `docs/search.md`.
- Linked the planned search roadmap from `README.md`.
- Added the repo convention that this changelog should stay brief and current.
