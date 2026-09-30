# Tasks: add-greeting-name

## Progress
- 2026-10-01 proposed; nothing built yet

## Decision log
- (none yet)

## 1. The greeting names a member
- [ ] 1.1 Read the display name in `src/home/greeting.ts`, with its test in `src/home/greeting.test.ts` citing [GREETING-1-S2]
- [ ] 1.2 Keep [GREETING-1-S1] passing for an anonymous visitor

## 2. The farewell
- [ ] 2.1 Show the farewell on sign-out in `src/auth/sign-out.ts`, tested in `src/auth/sign-out.test.ts` citing [FAREWELL-1-S1]
- [ ] 2.2 [FAREWELL-1-S2] is checked by hand in a real browser (manual: the sign-out redirect crosses the identity provider's domain)

- [ ] Slice: a signed-in member can see their name in the greeting at /
