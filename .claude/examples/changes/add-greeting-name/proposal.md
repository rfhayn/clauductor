**Status:** awaiting approval
**Roadmap row:** 1.1
**Risk:** low
**Budget:** $15

## Why
Signed-in members are greeted exactly like strangers, so the home page never shows that it knows
who they are. Greeting them by name is the smallest step towards a personal home page.

## What changes
A signed-in member sees their own name in the greeting, and a member who signs out sees a farewell.

## What the existing specs already guarantee
`greeting` requirement 1 greets every visitor with "Hello!". This change MODIFIES it (a member's
greeting carries their name) and ADDS the `farewell` capability.

## Out of scope
Localised greetings: roadmap row 1.4.

## How we'll know
- **Signal:** the share of members who open their profile from the home page rises.
- **Check after:** 30 days
