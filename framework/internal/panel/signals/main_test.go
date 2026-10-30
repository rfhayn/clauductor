package signals

// testgit's init drops a git hook's GIT_DIR, GIT_WORK_TREE and GIT_INDEX_FILE from this test
// binary before any of its code runs, so no process these tests start reaches the hook's
// repository (#302). TestEveryTestPackageImportsTestgit requires the import.
import _ "github.com/rfhayn/stridal/internal/testgit"
