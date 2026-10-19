Hand-written transcript fixtures for `transcript_test.go`. They hold no real transcript content:
every message body is a placeholder, and only the fields the reader decodes (D2) carry values the
tests check. Every fixture's session id is the one synthetic UUID in its file name, and no file
spells it out: `TestFixturesAreScrubbed` allows only `syntheticID` ids in a fixture's content.
