// Command terraform-provider-fixture is a stand-in provider binary that the
// release workflow's dry-run builds, packages and signs.
package main

import (
	"fmt"

	"github.com/google/uuid"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	// A real dependency gives the security scan a license to detect.
	fmt.Println(version, commit, uuid.Nil)
}
