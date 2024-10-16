// Package executes Launchr application.
package main

import (
	"github.com/launchrctl/launchr"

	_ "github.com/launchrctl/compose"
)

func main() {
	launchr.RunAndExit()
}
