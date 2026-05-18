package main

import "github.com/brendanv/float/internal/migrate"

// migrations is the canonical ordered list of one-time data migrations.
// Add new migrations at the end. Never remove or reorder existing entries.
var migrations = []migrate.Migration{}
