package brand

import (
	"testing"

	"github.com/clauductor/clauductor/internal/testgit"
)

// A git hook's GIT_DIR, GIT_WORK_TREE and GIT_INDEX_FILE reach no process these tests start (#302).
func TestMain(m *testing.M) { testgit.Main(m) }
