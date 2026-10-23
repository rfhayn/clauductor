## MODIFIED Requirements
### Requirement: A stall interrupts once per stretch, on the Mac and through the owner's push command
WHEN a lane's `stalled` alert first becomes active and is not approximate THE SYSTEM SHALL send it through the notifier's rules (once per stretch, grouped per lane, at most once per lane per `min_interval_seconds`, never while the lane's terminal has focus in a visible page, and not again after a restart while it is still active, and nothing at all while `alerts.notify` is false) as a macOS notification and, WHERE `~/.clauductor/panel/push.json` holds a valid push command, by running that command's argv with the title and body as two more arguments, with no shell, under a 10 s timeout past which it is stopped and counted as failed; THE SYSTEM SHALL read `push.json` at the start and again within 10 s of a change to it or to its folder, a change of mode or owner included; THE SYSTEM SHALL NOT run a push command whose file is not the user's, is writable by group or others or sits in a folder that is, or names a program by a relative path, SHALL NOT push any alert but `stalled` and the catch-up notices of `panel-catch-up` (each sent once, as a macOS notification and through the same command, file and timeout rules, and nothing at all while `alerts.notify` is false, except the notices of the panel's own rebuild, which go whatever it says and are titled "clauductor panel"), and SHALL build the push's title and body from the `stalled` alert's own fields alone, or from the catch-up notice's own text, the title naming the project by its config's `name` only while that config is trusted, and as "clauductor panel" otherwise.

#### Scenario: [STALL-3-S1] A new stall sends one notification and one push
- **GIVEN** a valid `push.json` whose `argv` is a stand-in program that records its arguments, and a lane that stalls at 00:30
- **WHEN** the notifier runs at 00:30, 00:31 and 00:45
- **THEN** one macOS notification and one push go out at 00:30, and none after
- **AND** the push's last two arguments are the title, naming the project and the lane, and a body naming the quiet time, "#152" and the kind of wait, and holding no task description and no last line
- **AND** when the same notice also carries a `waiting` alert whose text is "wants to use Bash", the push's body does not contain it

#### Scenario: [STALL-3-S2] A focused terminal suppresses the stall's notification and push
- **GIVEN** a lane that stalls while its terminal has keyboard focus in a visible page
- **WHEN** the notifier runs
- **THEN** no notification and no push go out, and the stall counts as seen

#### Scenario: [STALL-3-S3] An approximate stall never interrupts
- **GIVEN** a lane whose `stalled` alert is approximate
- **WHEN** the notifier runs
- **THEN** no notification and no push go out

#### Scenario: [STALL-3-S4] A push file that is unsafe or malformed never runs
- **GIVEN** in turn a `push.json` writable by group, one in a folder writable by group, one whose `argv[0]` is `ntfy`, and one that is not a JSON object
- **WHEN** a lane stalls
- **THEN** the push program is never run, the macOS notification goes, and a warning names the file and why it was refused
- **AND** with no `push.json` at all, the macOS notification goes and no warning shows

#### Scenario: [STALL-3-S5] A failed push is counted and does not stop the notification
- **GIVEN** a valid `push.json` whose program exits 1 with "no route"
- **WHEN** a lane stalls
- **THEN** the macOS notification goes, the notifier's failed count rises by one, and its error names "no route"

#### Scenario: [STALL-3-S6] A waiting alert is not pushed, and a stall after it is
- **GIVEN** a valid `push.json`, `waiting_seconds` 120 and `stall_minutes` 30, and a session that waits on a permission prompt from 00:00
- **WHEN** the notifier runs at 00:02 and at 00:30
- **THEN** at 00:02 a macOS notification goes for the waiting alert and no push
- **AND** at 00:30 the lane's stall goes out as a notification and a push

#### Scenario: [STALL-3-S7] A restart does not notify a stall still active
- **GIVEN** a stall resting on a gate receipt that was notified, and the panel restarted with the lane still stalled from the same saved last movement
- **AND** `gh pr list` and the merged read answer only after the notifier's first ticks have run
- **WHEN** the notifier runs before those reads answer, and again after
- **THEN** no notification and no push go out, and the stall's mark is still held in `notifier.json` throughout

#### Scenario: [STALL-3-S8] A push that does not answer is stopped at its timeout
- **GIVEN** a push command that never answers
- **WHEN** the panel runs it
- **THEN** it is stopped at its timeout, with an error saying it gave no answer within it

#### Scenario: [STALL-3-S9] A push file is read again when its mode changes
- **GIVEN** a running panel whose `push.json` is refused as writable by group, a warning naming the file and why
- **WHEN** the file's mode alone is changed to 0600
- **THEN** the warning goes, with no restart

#### Scenario: [STALL-3-S10] With notifications off, a stall sends nothing
- **GIVEN** `alerts.notify` false, a valid `push.json` whose `argv` is a stand-in program that records its arguments, and a lane with pull request #152 open that stalls
- **WHEN** the notifier runs
- **THEN** no macOS notification and no push go out
- **AND** the lane is still shown stalled

#### Scenario: [STALL-3-S11] An untrusted config's name never titles a push
- **GIVEN** a project whose `panel.json` names it "Stalls" and is not trusted, a valid `push.json` whose `argv` is a stand-in program that records its arguments, and a lane with pull request #152 open that stalls
- **WHEN** the notifier runs
- **THEN** one macOS notification and one push go out, each titled with "clauductor panel" and the lane, and neither title names "Stalls"

#### Scenario: [STALL-3-S12] A catch-up notice is pushed, and a waiting alert still is not
- **GIVEN** a valid `push.json` whose `argv` is a stand-in program that records its arguments, a project named "Panel" with catch-up on and a trusted config, and a session that has waited on a permission prompt past `waiting_seconds`
- **WHEN** the panel fast-forwards the project's main checkout to the owner's merged pull request #7, which changes its `panel.json`, and the notifier runs
- **THEN** the catch-up notice goes out once as a macOS notification and once as a push, titled with "Panel" and with a body naming "#7"
- **AND** the waiting alert goes out as a macOS notification and no push
