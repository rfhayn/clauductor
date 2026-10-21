## MODIFIED Requirements
### Requirement: Init and attach write the default policy
WHEN `panel init` or `panel attach` writes a config THE SYSTEM SHALL write `"launch": {"context_window": 200000, "claudeai_connectors": false}` and declare the latest config version; and a config that declares a version before 6 SHALL NOT use the policy's keys.

#### Scenario: [LANECTX-3-S1] An attached repository's lanes launch with the policy
- **GIVEN** a git repository that is not registered
- **WHEN** the owner attaches it and starts a lane in it
- **THEN** the sidecar declares the latest config version and holds the default `launch` object
- **AND** the lane's argv holds the `--settings` of LANECTX-1-S5

#### Scenario: [LANECTX-3-S2] A version-5 config cannot use the policy's keys
- **GIVEN** a config declaring `"version": 5` that sets `launch.context_window`
- **WHEN** the panel loads it
- **THEN** the load fails, and the error names the key and version 6
