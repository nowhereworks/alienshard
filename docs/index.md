# Alien Shard

![Alien Shard logo](alien-shard.png)

Alien Shard is a lightweight Go server for mixed human and machine Markdown workflows.

- Humans using Chrome or Firefox get rendered HTML for `.md` files.
- Machines, agents, curl, and scripts get raw Markdown for `.md` files.
- One process serves raw source files and a writable wiki layer.

## Documentation

- [HTTP API](http-api.md): routes, methods, content negotiation, status codes, and path validation.
- [Configuration](configuration.md): flags, environment variables, Docker defaults, and image tags.
- [Wiki](wiki.md): wiki storage, mutations, autoindex, and manual index behavior.
- [Development](development.md): build, test, smoke, release, and documentation workflow.
- [LLM Wiki](llm-wiki.md): product concept and LLM-maintained wiki pattern.
- [Search](search.md): search design, implementation notes, and future roadmap.
