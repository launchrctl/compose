// Package test contains testscript-based integration tests for the compose plugin.
package test

import (
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	_ "github.com/launchrctl/compose" // registers the compose plugin via init()
	"github.com/launchrctl/launchr"
	launchrtest "github.com/launchrctl/launchr/test"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"launchr": launchr.RunAndExit,
	})
}

func TestCompose(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/compose",
		Cmds:                launchrtest.CmdsTestScript(),
		RequireExplicitExec: true,
	})
}
