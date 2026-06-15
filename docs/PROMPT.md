# Next-iteration prompt

Copy the block below into a fresh agent session to advance Cairn by exactly one
iteration. It is written to keep token usage low: read the minimum, do one slice, stop.

---

````text
You are advancing the Cairn project by ONE iteration. Work token-frugally: read only what
is listed, edit precisely, do not scan the repo or re-read whole files unnecessarily.

1. Read AGENTS.md once (skip if already read this session).
2. Open docs/ITERATIONS.md and find the FIRST iteration not marked [x]. That is YOUR
   iteration. Mark it [~] (in progress).
3. Read ONLY the files named in that iteration's "Read:" line. Do not open anything else
   unless implementing reveals a genuine need — if so, add that file to the iteration's
   Read: line so the next run benefits.
4. Implement the iteration's Steps until its Acceptance criteria are met. Follow the
   principles in AGENTS.md: never reinvent a tool (shell out), keep it simple, respect the
   bounded contexts/ports in ARCHITECTURE, add MEANINGFUL tests only.
5. Definition of done (all required):
   - Code builds; the iteration's Acceptance criteria pass; new/updated tests are green.
   - Update CHANGELOG.md [Unreleased] with a one-line entry.
   - Tick the iteration to [x] in docs/ITERATIONS.md.
   - Propose a Conventional Commit message. Do NOT commit unless asked.
6. STOP after one iteration. Do not start the next one.

If the iteration is too large to finish cleanly, split it in docs/ITERATIONS.md into
smaller sub-iterations (e.g. 5a, 5b), complete the first, and stop.
````

---

## Tips

- Run this prompt repeatedly; it always picks up the next unfinished iteration.
- If you want a specific iteration, say so explicitly (e.g. "do iteration 5c").
- Keep ITERATIONS.md entries small; that is what keeps each run cheap.
