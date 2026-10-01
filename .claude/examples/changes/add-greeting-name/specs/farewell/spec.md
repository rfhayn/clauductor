## Purpose
Says goodbye to a member who signs out, so they can tell that signing out worked.

## ADDED Requirements

### Requirement: Say goodbye on sign-out
WHEN a member signs out THE SYSTEM SHALL show a farewell that names them.

#### Scenario: [FAREWELL-1-S1] A member signs out
- **GIVEN** a member signed in with the display name "Ana"
- **WHEN** they sign out
- **THEN** the page shows "Goodbye, Ana"

#### Scenario: [FAREWELL-1-S2] The farewell survives the identity provider's redirect
- **GIVEN** a member who signed in through the identity provider
- **WHEN** they sign out and the provider redirects back
- **AND** the redirect takes more than a second
- **THEN** the page still shows "Goodbye, <name>"
