module github.com/winfsp/hubfs

go 1.14

require (
	github.com/billziss-gh/golib v0.2.0
	github.com/cli/browser v1.0.0
	github.com/cli/oauth v0.9.0
	github.com/go-git/go-git/v5 v5.2.0
	github.com/winfsp/cgofuse v1.6.0
)

replace github.com/go-git/go-git/v5 v5.2.0 => github.com/billziss-gh/go-git/v5 v5.2.1-0.20210325075736-c1624bffeb12
