# Design: add-greeting-name

## Decisions
- **D1 Where the name comes from.** The member's display name, not their email: an email in the
  greeting leaks it to anyone looking at the screen. Beat: the email's local part.

## Refusals
| Situation | What happens instead |
|---|---|
| A member with no display name | The anonymous greeting, "Hello!" |
