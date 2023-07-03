// Package executes Launchr application.
package main

import (
	"os"

	"github.com/launchrctl/launchr"

	_ "github.com/launchrctl/compose"
)

func main() {
	os.Exit(launchr.Run())
}
