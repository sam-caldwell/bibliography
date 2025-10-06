# Contributor Guidelines (Civility First)

Welcome — we’re glad you’re here. This project values civility, clarity, and collaboration. These guidelines explain how to participate constructively and how we handle conduct and contributions.

## 1) Civility & Conduct

We commit to a respectful, inclusive environment for contributors of all backgrounds and experience levels.

Always
- Be respectful and assume good intent.
- Be clear and kind in language; prefer curiosity over certainty.
- Welcome questions; mentor where possible.
- Respect time zones and boundaries; be patient for replies.

Never
- No harassment, personal attacks, or insults.
- No discrimination or exclusionary behavior.
- No doxxing or sharing private information.
- No aggressive debate or trolling; disagree without being disagreeable.

If conflict arises
- De‑escalate and seek common ground.
- Move sensitive discussions out of public threads when appropriate.
- If needed, ask a maintainer to moderate.

## 2) Reporting Concerns

If you experience or witness behavior that violates these guidelines:
- Contact the project maintainers (repository Owners) via their GitHub profile.
- If unsure how to reach out privately, open a minimal issue titled “[MODERATION]” describing the concern at a high 
  level, and request maintainer contact. Do not include sensitive details publicly.
- In emergencies or for platform abuse, also use GitHub’s reporting tools.

Maintainers will review in good faith, keep reports as confidential as possible, and take proportionate action
(warning, content moderation, or removal from the project in severe cases).

## 3) How to Contribute

- Issues: Use clear titles, steps to reproduce, expected/actual behavior. Be concise.
- Pull Requests:
  - Keep PRs focused and reasonably small.
  - Explain the “why” and “what changed” in the description.
  - Link related issues. Note breaking changes clearly.
  - Be open to review feedback; requested changes are part of collaboration.

## 4) Code & Quality

- Language: Go 1.22+
- Style: Run `gofmt`/`go fmt` (CI enforces formatting).
- Tests: Add/adjust tests to keep total coverage ≥ 80%.
- Scope: Match the repository’s architecture and specification (`specification-*.yaml`).
- Security/Privacy: Never exfiltrate repository contents in AI prompts beyond explicit user hints.

## 5) Commit & Branching

- Commits: Write clear, imperative messages (e.g., “add article support”).
- Branching: Use feature branches; keep main green.
- Git: The CLI auto‑commits/pushes for `add`; CI may commit metadata on push.

## 6) Reviews

- Review the change, not the person. Focus on correctness, clarity, and risk.
- Prefer specific suggestions over vague critique.
- It’s okay to say “I don’t know” and ask for a second opinion.

## 7) Decision-Making

- Small changes: handled in PRs after review.
- Larger changes: propose via an issue with rationale and alternatives.
- Maintainers are responsible for final decisions and balancing scope vs. goals.

## 8) Attribution & Credit

- Acknowledge contributions in PR descriptions and commit messages.
- We value both code and non-code contributions (docs, tests, triage, design).

## 9) Enforcement

Maintainers may take action for behavior that violates these guidelines, including:
- Private or public warnings
- Editing/removing problematic content
- Temporarily or permanently restricting participation

Appeals can be made privately to the maintainers. We aim to be fair, transparent, and proportionate.

By participating, you agree to uphold these principles. Thank you for contributing with civility and care.
