module github.com/absfs/fusefs/examples/basic

go 1.24.0

replace github.com/absfs/fusefs => ../..

require (
	github.com/absfs/absfs v0.0.0-20251109181304-77e2f9ac4448
	github.com/absfs/fusefs v0.0.0-00010101000000-000000000000
)

require (
	github.com/hanwen/go-fuse/v2 v2.9.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
)
