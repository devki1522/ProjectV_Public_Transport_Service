# Installed design skills
Installed 2026-11-19 into the project-local .agents/skills directory using the Codex
skill-installer GitHub downloader (noninteractive; full skill support files preserved).

- addyosmani/agent-skills: skills/frontend-ui-engineering
- anthropics/skills: skills/frontend-design
- jakubkrehel/skills: all 16 skills under skills/

The frontend redesign applies frontend-ui-engineering, frontend-design and the seven
better-* domain rules. Workflow tools such as afk/upskill/variant are installed but not
invoked: their separate workflows are not needed for the requested implementation.
User requirements take precedence over skill defaults. See frontend/DESIGN.md for the
project design contract, reference sources and verification boundaries.
