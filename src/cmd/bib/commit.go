package main

import "bibliography/src/internal/gitutil"

// indirection for testability across commands
var commitAndPush = gitutil.CommitAndPush

