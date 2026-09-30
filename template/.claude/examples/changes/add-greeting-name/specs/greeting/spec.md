## MODIFIED Requirements

### Requirement: Greet every visitor
The system SHALL show a greeting on the home page to every visitor, naming a signed-in member.

#### Scenario: [GREETING-1-S1] An anonymous visitor is greeted
- **GIVEN** a visitor who is not signed in
- **WHEN** they open the home page
- **THEN** the page shows "Hello!"

#### Scenario: [GREETING-1-S2] A member is greeted by name
- **GIVEN** a member signed in with the display name "Ana"
- **WHEN** they open the home page
- **THEN** the page shows "Hello, Ana!"
